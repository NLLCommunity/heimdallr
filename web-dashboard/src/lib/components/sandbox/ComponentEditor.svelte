<script lang="ts">
  import type { ComponentNode } from "./serialize";
  import TextDisplayEditor from "./TextDisplayEditor.svelte";
  import SectionEditor from "./SectionEditor.svelte";
  import ContainerEditor from "./ContainerEditor.svelte";
  import SeparatorEditor from "./SeparatorEditor.svelte";
  import MediaGalleryEditor from "./MediaGalleryEditor.svelte";
  import ActionRowEditor from "./ActionRowEditor.svelte";

  interface Props {
    node: ComponentNode;
  }

  let { node }: Props = $props();
</script>

{#if node.type === "text_display"}
  <TextDisplayEditor {node} />
{:else if node.type === "section"}
  <SectionEditor {node} />
{:else if node.type === "container"}
  <ContainerEditor {node} />
{:else if node.type === "separator"}
  <SeparatorEditor {node} />
{:else if node.type === "media_gallery"}
  <MediaGalleryEditor {node} />
{:else if node.type === "action_row"}
  <ActionRowEditor {node} />
{/if}
