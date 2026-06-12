import { HttpClient } from "../http";

export class HealthApi {
  constructor(private readonly http: HttpClient) {}

  async ping(): Promise<string> {
    return this.http.requestText("/health");
  }
}
