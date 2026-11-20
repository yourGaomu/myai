// Web history uses AsyncStorage, so Metro does not need the SQLite WASM bundle.
export function importSessionHistorySQLite(): Promise<never> {
  return Promise.reject(new Error("SQLite history cache is disabled on web"));
}
