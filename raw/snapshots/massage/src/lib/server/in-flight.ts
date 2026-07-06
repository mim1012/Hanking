export function getSharedInFlight<T>(
  store: Map<string, Promise<T>>,
  key: string,
  load: () => Promise<T>,
) {
  const activeRequest = store.get(key);
  if (activeRequest) {
    return activeRequest;
  }

  const request = load().finally(() => {
    store.delete(key);
  });
  store.set(key, request);
  return request;
}
