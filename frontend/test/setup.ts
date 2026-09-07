import '@testing-library/jest-dom/vitest'

// Node 26 exposes an incomplete experimental localStorage unless a backing
// file is configured. Give jsdom tests the browser contract they exercise.
const values = new Map<string, string>()
const memoryStorage: Storage = {
  get length() { return values.size },
  clear: () => values.clear(),
  getItem: key => values.get(key) ?? null,
  key: index => [...values.keys()][index] ?? null,
  removeItem: key => { values.delete(key) },
  setItem: (key, value) => { values.set(key, String(value)) },
}
Object.defineProperty(window, 'localStorage', {
  configurable: true,
  value: memoryStorage,
})
