// Replaces @joint/router-avoid's WorkerProvider in the build (vite.config.ts):
// dg runs libavoid on the main thread, and the real provider would make Vite
// bundle an unused worker script.
export class WorkerProvider {
  constructor() {
    throw new Error('router-avoid worker mode is not bundled');
  }
}
