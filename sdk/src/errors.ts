export class ApiError extends Error {
  constructor(
    public status: number,
    public statusText: string,
    public body?: any,
  ) {
    super(`${status} ${statusText}`);
    this.name = "ApiError";
  }
}
