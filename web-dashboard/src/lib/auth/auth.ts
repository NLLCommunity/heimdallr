import { userStore } from "../stores/user.svelte";

export function isLoggedIn(): boolean {
  return userStore().isLoggedIn;
}
