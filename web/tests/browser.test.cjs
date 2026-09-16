const { test, before, after } = require('node:test');
const assert = require('node:assert/strict');
const { chromium, expect } = require('@playwright/test');
const { execFileSync } = require('node:child_process');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const repo = path.resolve(__dirname, '../..');
let browser, fixtures;

before(async () => {
  fixtures = fs.mkdtempSync(path.join(os.tmpdir(), 'heimdallr-editors-'));
  execFileSync('go', ['run', './web/tests/fixtures.go', fixtures], { cwd: repo, stdio: 'inherit' });
  browser = await chromium.launch({ headless: true, executablePath: process.env.CHROMIUM_PATH || undefined });
});
after(async () => {
  await browser?.close();
  if (fixtures) fs.rmSync(fixtures, { recursive: true, force: true });
});

async function open(name, options = {}) {
  const page = await browser.newPage();
  const errors = [], requests = [], submissions = [];
  page.on('pageerror', error => errors.push(error.message));
  await page.route('**/*', async route => {
    const req = route.request(), url = new URL(req.url());
    requests.push(url.pathname);
    if (url.origin !== 'http://editor.test') return route.fulfill({
      contentType:'text/css', headers:{'access-control-allow-origin':'*'},
      body:process.env.PICO_CSS ? fs.readFileSync(process.env.PICO_CSS) : '',
    });
    if (url.pathname.endsWith('/components.mjs') && options.failBundle) return route.abort();
    if (url.pathname.startsWith('/static/')) {
      const filename = path.join(repo, 'web', url.pathname);
      return route.fulfill({ contentType:filename.endsWith('.css') ? 'text/css' : 'text/javascript', body:fs.readFileSync(filename) });
    }
    if (req.method() === 'POST') {
      const body = Object.fromEntries(new URLSearchParams(req.postData()));
      submissions.push({ path:url.pathname, body });
      if (url.pathname.endsWith('/preview')) return route.fulfill({ contentType:'text/html', body:'<p>Publish check complete</p>' });
      if (url.pathname.endsWith('/load')) return route.fulfill({ json:options.v1
        ? {is_v2:false, channel_id:'123', message_id:'456', content:'Legacy content'}
        : { is_v2:true, channel_id:'123', message_id:'456', components:[{type:10,content:'Loaded message',id:19}] } });
      if (url.pathname.includes('/settings/')) return route.fulfill({ contentType:'text/html', body:fs.readFileSync(path.join(fixtures,url.pathname.endsWith('/birthday') ? 'birthday-fragment.html' : 'settings-fragment.html')) });
      if (url.pathname.endsWith('/send') || url.pathname.endsWith('/edit')) return route.fulfill({contentType:'text/html', body:'Sent'});
      return route.fulfill({ json:{version:3} });
    }
    return route.fulfill({ contentType:'text/html', body:fs.readFileSync(path.join(fixtures, name+'.html')) });
  });
  await page.goto('http://editor.test/'+name);
  return { page, errors, requests, submissions };
}

async function ready(page, count) {
  await expect(page.locator('[data-component-editor][data-editor-state="ready"]')).toHaveCount(count);
}

test('post editing, stable message reorder and save preserve explicit JSON', async () => {
  const {page, errors, requests, submissions} = await open('post');
  try {
    await ready(page, 2);
    const cards = page.locator('.post-message');
    await cards.nth(0).locator('textarea').fill('Edited first message');
    await expect(page.getByRole('button', {name:'Publish / Update on Discord'})).toBeDisabled();
    await cards.nth(1).getByRole('button', {name:'Move message up'}).click();
    await expect(cards.nth(0).locator('textarea')).toHaveValue('Second message');
    await expect(cards.nth(1).locator('textarea')).toHaveValue('Edited first message');
    await page.getByRole('button', {name:'Save',exact:true}).click();
    await expect(page.getByText('Saved.', {exact:true})).toBeVisible();
    const save = submissions.find(req => req.path === '/guild/1/posts/1');
    assert.deepEqual(JSON.parse(save.body.components_json), {version:1,messages:[{components:[{type:10,content:'Second message'}]},{components:[{type:10,content:'Edited first message',id:15}]}]});
    await expect(page.getByRole('button', {name:'Publish / Update on Discord'})).toBeEnabled();
    await page.getByRole('button', {name:'Add message',exact:true}).click();
    await ready(page,3);
    await cards.nth(2).getByRole('button', {name:'Remove',exact:true}).click();
    await ready(page,2);
    assert.equal(requests.filter(x=>x.endsWith('/components.mjs')).length,1);
    assert.deepEqual(errors,[]);
  } finally { await page.close(); }
});

test('settings synchronize hidden inputs and dirty state through an HTMX replacement', async () => {
  const {page,errors,submissions} = await open('settings');
  try {
    await ready(page,2);
    const gate = page.locator('#gatekeep');
    assert.equal(await gate.locator('discord-message-editor').evaluate(el=>el.form), null);
    await gate.locator('discord-message-editor textarea').fill('Updated {{user}}');
    await expect(gate.getByText('Unsaved changes', {exact:true})).toBeVisible();
    await gate.locator('discord-message-editor textarea').fill('Hello {{user}}');
    await expect(gate.getByText('Unsaved changes', {exact:true})).toBeHidden();
    await gate.locator('discord-message-editor textarea').fill('Updated {{user}}');
    await gate.getByRole('button', {name:'Save',exact:true}).click();
    await expect.poll(()=>submissions.filter(x=>x.path.endsWith('/gatekeep')).length).toBe(1);
    assert.deepEqual(JSON.parse(submissions.find(x=>x.path.endsWith('/gatekeep')).body.approved_message_v2_json),[{type:10,content:'Updated {{user}}',id:17}]);
    await ready(page,2);
    await expect(gate.locator('discord-message-editor textarea')).toHaveValue('Hello {{user}}');
    await gate.locator('discord-message-editor textarea').fill('Edited after swap');
    await expect(gate.getByText('Unsaved changes',{exact:true})).toBeVisible();
    // A loaded, invalid empty Lit draft must not block its V2-off form either.
    await page.locator('#birthday').getByRole('button',{name:'Save',exact:true}).click();
    await expect.poll(()=>submissions.filter(x=>x.path.endsWith('/birthday')).length).toBe(1);
    assert.deepEqual(errors,[]);
  } finally {await page.close();}
});

test('failed bundle blocks V2 saves but does not block V2-off settings', async () => {
  const {page,submissions,errors} = await open('settings',{failBundle:true});
  try {
    await expect(page.locator('[data-editor-state="error"]')).toHaveCount(2);
    await expect(page.locator('#birthday [x-bind\\:data-editor-enabled]')).toHaveAttribute('data-editor-enabled','false');
    await page.locator('#gatekeep').getByRole('button',{name:'Save',exact:true}).click();
    assert.equal(submissions.length,0);
    await page.locator('#birthday').getByRole('button',{name:'Save',exact:true}).click();
    await expect.poll(()=>submissions.length).toBe(1);
    assert.match(submissions[0].path,/birthday$/);
    assert.deepEqual(errors,[]);
  } finally {await page.close();}
});

test('sandbox loads and edits V2 JSON without losing component IDs', async () => {
  const {page,submissions,errors} = await open('sandbox');
  try {
    await ready(page,1);
    await page.getByPlaceholder('https://discord.com/channels/.../.../...').fill('https://discord.com/channels/1/123/456');
    await page.getByRole('button',{name:'Load',exact:true}).click();
    await expect(page.locator('discord-message-editor textarea')).toHaveValue('Loaded message');
    await page.locator('discord-message-editor textarea').fill('Sandbox edit');
    await page.getByRole('button',{name:'Update Message',exact:true}).click();
    await expect.poll(()=>submissions.filter(x=>x.path.endsWith('/edit')).length).toBe(1);
    const body=submissions.find(x=>x.path.endsWith('/edit')).body;
    assert.equal(body.message_id,'456');
    assert.deepEqual(JSON.parse(body.components_json),[{type:10,content:'Sandbox edit',id:19}]);
    assert.deepEqual(errors,[]);
  } finally {await page.close();}
});

test('V1 sandbox editing still works when the V2 bundle fails', async () => {
  const {page,submissions,errors} = await open('sandbox',{failBundle:true,v1:true});
  try {
    await expect(page.locator('[data-editor-state="error"]')).toHaveCount(1);
    await page.getByPlaceholder('https://discord.com/channels/.../.../...').fill('https://discord.com/channels/1/123/456');
    await page.getByRole('button',{name:'Load',exact:true}).click();
    await expect(page.getByPlaceholder('Message content')).toHaveValue('Legacy content');
    await page.getByPlaceholder('Message content').fill('Legacy edit');
    await page.getByRole('button',{name:'Update Message',exact:true}).click();
    await expect.poll(()=>submissions.filter(x=>x.path.endsWith('/edit')).length).toBe(1);
    assert.equal(submissions.find(x=>x.path.endsWith('/edit')).body.content,'Legacy edit');
    assert.equal(submissions.find(x=>x.path.endsWith('/edit')).body.is_v2,'false');
    assert.deepEqual(errors,[]);
  } finally {await page.close();}
});

test('message editor and actions fit a narrow viewport', async () => {
  const {page,errors} = await open('post');
  try {
    await page.setViewportSize({width:390,height:844});
    await ready(page,2);
    if (process.env.EDITOR_SCREENSHOT) await page.screenshot({path:process.env.EDITOR_SCREENSHOT,fullPage:true});
    const layout = await page.evaluate(() => {
      const overflowing = [];
      const inspect = root => {
        for (const el of root.querySelectorAll('*')) {
          if (el.getBoundingClientRect().right > window.innerWidth) overflowing.push(el.tagName + '.' + el.className);
          if (el.shadowRoot) inspect(el.shadowRoot);
        }
      };
      // Scope to the changed editor layout. The CDN stylesheet is mocked in
      // offline tests, so unrelated site navigation has browser default CSS.
      const workspace = document.querySelector('.message-workspace');
      inspect(workspace);
      return { right:workspace.getBoundingClientRect().right, viewport:window.innerWidth, overflowing };
    });
    assert.ok(layout.right <= layout.viewport && layout.overflowing.length === 0,JSON.stringify(layout));
    await expect(page.getByRole('button',{name:'Add message',exact:true})).toBeVisible();
    assert.deepEqual(errors,[]);
  } finally {await page.close();}
});
