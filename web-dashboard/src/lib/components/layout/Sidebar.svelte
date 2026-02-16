<script lang="ts">
  const sections = [
    { id: "mod-channel", label: "Moderator Channel" },
    { id: "infractions", label: "Infractions" },
    { id: "gatekeep", label: "Gatekeep" },
    { id: "join-leave", label: "Join/Leave Messages" },
    { id: "anti-spam", label: "Anti-Spam" },
    { id: "ban-footer", label: "Ban Footer" },
    { id: "modmail", label: "Modmail" },
  ];
</script>

<aside class="bg-base-200 hidden w-56 shrink-0 p-4 lg:block">
  <ul class="menu w-full">
    <li class="menu-title">Settings</li>
    {#each sections as section}
      <li>
        <a href="#{section.id}" class="text-sm">{section.label}</a>
      </li>
    {/each}
  </ul>
</aside>
