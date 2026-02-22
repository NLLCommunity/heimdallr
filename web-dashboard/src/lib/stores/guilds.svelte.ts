import type { Guild } from "../../gen/heimdallr/v1/auth_pb";

let list = $state<Guild[]>([]);
let loading = $state(false);

export function guildsStore() {
  return {
    get guilds() {
      return list;
    },
    set guilds(g: Guild[]) {
      list = g;
    },
    get loading() {
      return loading;
    },
    set loading(l: boolean) {
      loading = l;
    },
  };
}
