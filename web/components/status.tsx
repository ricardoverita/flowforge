export function Status({ value }: { value: string }) {
  return <span className={`status status-${value}`}>{value}</span>;
}

export function formatTime(value?: string | null): string {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.valueOf()) ? "—" : date.toISOString().replace("T", " ").replace(/\.\d{3}Z$/, " UTC");
}
