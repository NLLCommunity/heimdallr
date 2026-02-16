<script lang="ts">
  import Router from "svelte-spa-router";
  import Login from "./lib/routes/Login.svelte";
  import Callback from "./lib/routes/Callback.svelte";
  import GuildSelector from "./lib/routes/GuildSelector.svelte";
  import Dashboard from "./lib/routes/Dashboard.svelte";

  const routes = {
    "/": Login,
    "/callback": Callback,
    "/guilds": GuildSelector,
    "/guild/:id": Dashboard,
  };
</script>

<Router {routes} />
