export interface AdminMutationLease<TIdentity> {
  readonly signal: AbortSignal
  applyResult: <TValue, TResult>(value: TValue, apply: (value: TValue) => TResult) => TResult | undefined
  finish: () => void
  readonly identity: TIdentity
}

export interface AdminMutationCoordinator<TIdentity> {
  begin: (identity: TIdentity) => AdminMutationLease<TIdentity> | null
  beginSessionTransition: (identity?: TIdentity) => boolean
  endSessionTransition: () => void
}

export function createAdminMutationCoordinator<TIdentity>(isIdentityCurrent: (identity: TIdentity) => boolean): AdminMutationCoordinator<TIdentity> {
  const activeControllers = new Set<AbortController>()
  let sessionTransition = false

  return {
    begin: (identity) => {
      if (sessionTransition) return null
      const controller = new AbortController()
      activeControllers.add(controller)
      let finished = false
      return {
        identity,
        signal: controller.signal,
        applyResult: (value, apply) => {
          if (controller.signal.aborted || !isIdentityCurrent(identity)) return undefined
          return apply(value)
        },
        finish: () => {
          if (finished) return
          finished = true
          activeControllers.delete(controller)
        },
      }
    },
    beginSessionTransition: (identity) => {
      if (identity !== undefined && !isIdentityCurrent(identity)) return false
      sessionTransition = true
      for (const controller of activeControllers) controller.abort()
      return true
    },
    endSessionTransition: () => {
      sessionTransition = false
    },
  }
}

export async function runAdminMutationLease<TIdentity, TValue, TResult>(
  lease: AdminMutationLease<TIdentity>,
  operation: (signal: AbortSignal) => Promise<TValue>,
  apply: (value: TValue) => TResult,
): Promise<TResult | undefined> {
  try {
    const value = await operation(lease.signal)
    return lease.applyResult(value, apply)
  } finally {
    lease.finish()
  }
}
