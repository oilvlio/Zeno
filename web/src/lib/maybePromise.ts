export type MaybePromise<T = void> = T | Promise<T>

export function runMaybePromise<T>(action: () => MaybePromise<T>): Promise<T> {
  return Promise.resolve().then(action)
}
