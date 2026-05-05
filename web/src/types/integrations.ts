export const SECRET_NAMES = ["anthropic_api_key", "zernio_api_key"] as const;
export type SecretName = (typeof SECRET_NAMES)[number];

export type SecretMetadata = {
  name: SecretName;
  created_at: string;
  updated_at: string;
  kek_version: number;
  algorithm: string;
  decryptable: boolean;
};

export type ZernioState = "disabled" | "degraded" | "ok";

export type ZernioHealth = {
  enabled: boolean;
  state: ZernioState;
  profileId?: string;
  lastSyncAt?: string;
  lastSyncStatus?: string;
  accountCount: number;
};

export type ZernioAccount = {
  id: string;
  platform: string;
  username: string;
  displayName: string;
  avatarUrl: string;
  isActive: boolean;
  connectedAt: string;
  lastSyncedAt: string;
};

export type ZernioAccountsResponse = {
  accounts: ZernioAccount[];
  lastSyncAt?: string;
  lastSyncStatus?: string;
};

export type ConnectLinkResponse = {
  platform: string;
  connectUrl: string;
  expiresAt: string;
};

export type ZernioErrorCode =
  | "integration_disabled"
  | "integration_degraded"
  | "rate_limited"
  | "invalid_platform"
  | "unknown";

export class ZernioError extends Error {
  code: ZernioErrorCode;
  status: number;
  retryAfterSeconds?: number;

  constructor(code: ZernioErrorCode, status: number, message: string, retryAfterSeconds?: number) {
    super(message);
    this.name = "ZernioError";
    this.code = code;
    this.status = status;
    this.retryAfterSeconds = retryAfterSeconds;
  }
}
