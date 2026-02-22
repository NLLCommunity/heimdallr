import { equals, type Message } from "@bufbuild/protobuf";
import type { GenMessage } from "@bufbuild/protobuf/codegenv2";

export function isDirty<T extends Message>(schema: GenMessage<T>, data: T, saved: T): boolean {
  return !equals(schema, data, saved);
}
