export function newRequestID() {
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

export function shortID(value?: string) {
  if (!value) {
    return "-";
  }
  return value.length > 8 ? value.slice(0, 8) : value;
}
