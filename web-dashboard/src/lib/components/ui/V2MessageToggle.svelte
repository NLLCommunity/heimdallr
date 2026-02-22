<script lang="ts">
  import TextareaField from "./TextareaField.svelte";
  import MessageBuilder from "../sandbox/MessageBuilder.svelte";
  import MessagePreview from "../sandbox/MessagePreview.svelte";
  import { deserializeComponents, serializeComponents, type ComponentNode } from "../sandbox/serialize";

  interface Props {
    label: string;
    description?: string;
    helpText: string;
    plainText: string;
    v2Enabled: boolean;
    v2Json: string;
    onPlainTextChange: (v: string) => void;
    onV2EnabledChange: (v: boolean) => void;
    onV2JsonChange: (v: string) => void;
  }

  let {
    label,
    description,
    helpText,
    plainText,
    v2Enabled,
    v2Json,
    onPlainTextChange,
    onV2EnabledChange,
    onV2JsonChange,
  }: Props = $props();

  let modalRef: HTMLDialogElement | undefined = $state();
  let components: ComponentNode[] = $state([]);

  function componentCount(): number {
    try {
      const parsed = JSON.parse(v2Json || "[]");
      return Array.isArray(parsed) ? parsed.length : 0;
    } catch {
      return 0;
    }
  }

  function openEditor() {
    components = deserializeComponents(v2Json);
    modalRef?.showModal();
  }

  function closeEditor() {
    onV2JsonChange(serializeComponents(components));
    modalRef?.close();
  }
</script>

<div class="flex flex-col gap-2">
  <div class="flex flex-col">
    <span class="text-sm font-semibold">{label}</span>
    {#if description}
      <span class="text-base-content/60 text-xs">{description}</span>
    {/if}
  </div>

  <label class="flex cursor-pointer items-center gap-3">
    <input
      type="checkbox"
      class="toggle toggle-sm toggle-primary"
      checked={v2Enabled}
      onchange={(e) => onV2EnabledChange(e.currentTarget.checked)}
    />
    <span class="text-sm">Use Components V2</span>
  </label>

  {#if v2Enabled}
    <div class="bg-base-200 flex items-center justify-between rounded-lg p-3">
      <span class="text-sm">
        {componentCount()} component{componentCount() !== 1 ? "s" : ""} configured
      </span>
      <button type="button" class="btn btn-primary btn-sm" onclick={openEditor}>
        Edit Components
      </button>
    </div>
  {:else}
    <TextareaField
      label=""
      value={plainText}
      {helpText}
      onchange={onPlainTextChange}
    />
  {/if}
</div>

<dialog bind:this={modalRef} class="modal">
  <div class="modal-box max-w-7xl h-[90vh] flex flex-col">
    <div class="flex items-center justify-between mb-4">
      <h3 class="text-lg font-bold">{label} - Components V2 Editor</h3>
      <button type="button" class="btn btn-sm btn-ghost" onclick={closeEditor}>
        Done
      </button>
    </div>
    <div class="flex-1 grid grid-cols-1 lg:grid-cols-2 gap-4 overflow-y-auto min-h-0">
      <div class="overflow-y-auto">
        <MessageBuilder bind:components />
      </div>
      <div class="overflow-y-auto">
        <MessagePreview {components} />
      </div>
    </div>
    <div class="modal-action">
      <button type="button" class="btn btn-primary" onclick={closeEditor}>
        Done
      </button>
    </div>
  </div>
  <form method="dialog" class="modal-backdrop">
    <button type="button" onclick={closeEditor}>close</button>
  </form>
</dialog>
