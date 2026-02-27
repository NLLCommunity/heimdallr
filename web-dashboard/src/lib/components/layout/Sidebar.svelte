<script lang="ts">
  import { sections } from "../../sections";

  function scrollTo(id: string) {
    document.getElementById(id)?.scrollIntoView({ behavior: "smooth" });
  }
</script>

<aside class="bg-base-200 hidden w-56 shrink-0 p-4 lg:block">
  <ul class="menu w-full">
    <li class="menu-title">Settings</li>
    {#each sections as section}
      <li>
        <button class="text-sm" onclick={() => scrollTo(section.id)}>{section.label}</button>
      </li>
    {/each}
  </ul>
</aside>
