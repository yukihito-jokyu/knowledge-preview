export function isPublicId(value: string): boolean {
  if (value.length !== 43 || !/^[A-Za-z0-9_-]+$/.test(value)) return false;
  try {
    const padding = "=".repeat((4 - (value.length % 4)) % 4);
    const decoded = atob(value.replaceAll("-", "+").replaceAll("_", "/") + padding);
    return decoded.length === 32;
  } catch {
    return false;
  }
}
