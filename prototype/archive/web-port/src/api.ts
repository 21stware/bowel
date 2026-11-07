// The services the page talks to. In the app they are the generated client
// (src/mygo.ts, the contract with Go); opened in a plain browser, main.ts
// installs the mock from src/mock.ts behind the same shape. Always call
// through `api.X.method(...)` so the installed implementation is used.

import { Activity, Control, Moho, Providers, Routes, events } from "./mygo";

export interface Api {
  Moho: typeof Moho;
  Control: typeof Control;
  Routes: typeof Routes;
  Providers: typeof Providers;
  Activity: typeof Activity;
  events: typeof events;
}

export const api: Api = { Moho, Control, Routes, Providers, Activity, events };

/** Replaces the services, e.g. with the mock outside MyGo. */
export function install(impl: Api): void {
  Object.assign(api, impl);
}
