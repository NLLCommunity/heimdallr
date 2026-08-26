# Bundle Retry Lifecycle Design

## Goal

Remove the runtime-dependent `unsafe` bundle identity mechanism while preserving one-time route installation and retryable Discord command synchronization.

## Problem

`RegisterAndSyncBundles` currently combines local route installation with remote command synchronization. After the first call installs routes, a transient synchronization failure must be retryable without installing the routes again.

The current retry API accepts the bundle functions again and attempts to prove that they are the exact same closures. Go function values are not comparable except to `nil`, so this proof relies on an undocumented runtime function-value representation through `unsafe`. That is not a stable language contract.

## API and Lifecycle

Keep `BundledInteractions` as its existing function type and separate registration from retry input:

```go
func (c *RaveClient) RegisterAndSyncBundles(
    guilds []snowflake.ID,
    interactions ...BundledInteractions,
) error

func (c *RaveClient) RegisterAndSyncBundlesGlobal(
    interactions ...BundledInteractions,
) error

func (c *RaveClient) RetrySyncBundles() error
```

The first `RegisterAndSyncBundles` call:

1. Acquires the existing bundle mutex.
2. Registers every bundle on the router exactly once.
3. Caches the generated command payload.
4. Copies and caches the requested guild scope.
5. Marks installation complete before attempting synchronization.
6. Synchronizes the cached payload.

If synchronization fails, the routes, command payload, and guild scope remain cached. The caller retries by calling `RetrySyncBundles()` with no bundle arguments.

`RetrySyncBundles`:

1. Acquires the same mutex, serializing retries.
2. Returns a stable error if no bundle installation has occurred.
3. Synchronizes the cached command payload to the cached guild scope.
4. Never mutates the router or accepts replacement bundles.

Any later call to either registration method returns a stable already-installed error before route mutation or REST synchronization. This is true whether the supplied bundles are identical or different. Replacing an installed bundle set is intentionally unsupported; it requires a new client.

The global registration wrapper caches a global scope. `RetrySyncBundles()` therefore needs no global variant and cannot accidentally retarget commands to a different guild scope.

## State

`RaveClient` retains:

- the existing mutex;
- an installation flag;
- the cached command payload;
- a copied guild-ID slice.

It no longer retains bundle functions, bundle counts, inferred identities, reflection pointers, or unsafe runtime pointers.

Export stable sentinel errors for retry-before-installation and repeated installation so callers can use `errors.Is` rather than matching strings.

## Concurrency

Registration and retry synchronization use the same mutex. Exactly one first registration can install routes; concurrent later registration attempts receive the already-installed error. Concurrent retries are serialized, preserving the existing no-overlapping-sync guarantee.

This change does not broaden the router's concurrency contract. Initial registration must still finish before callers directly access the router or begin external event dispatch.

## Tests

Regression coverage will prove:

- a failed first synchronization followed by `RetrySyncBundles` installs routes once and reuses the identical cached payload;
- guild-scoped and global retries preserve the initial scope;
- retry before installation returns the new sentinel and performs no REST work;
- repeated registration with the same or different bundles returns the already-installed sentinel and performs no route or REST work;
- concurrent retries remain serialized under the race detector;
- caller mutation of the original guild slice cannot alter retry scope.

Tests that inspect closure identity, closure lifetime, runtime pointers, or equivalent/reordered bundle closures will be removed because bundle identity is no longer part of the API.

## Compatibility

Normal one-time production registration is unchanged. Callers that previously retried by passing the same bundle arguments to `RegisterAndSyncBundles` must instead call `RetrySyncBundles`.

The `BundledInteractions` function type and `Bundle(...)` construction API remain unchanged, avoiding a broader migration to identity-bearing bundle objects.
