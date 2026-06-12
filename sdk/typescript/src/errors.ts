export class EvelentError extends Error {
  constructor(
    public readonly status: number,
    public readonly statusText: string,
    public readonly body?: unknown,
  ) {
    super(`${status} ${statusText}`);
    this.name = "EvelentError";
  }
}
