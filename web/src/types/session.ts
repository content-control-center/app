export type LoginPayload = {
  email: string;
  password: string;
};

export type Session = {
  id: string;
  user_id: string;
  expires_at: string;
};
