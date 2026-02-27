import type { User } from "../../gen/heimdallr/v1/auth_pb";

let current = $state<User | null>(null);

export function userStore() {
  return {
    get user() {
      return current;
    },
    set user(u: User | null) {
      current = u;
    },
    get isLoggedIn() {
      return current !== null;
    },
  };
}
