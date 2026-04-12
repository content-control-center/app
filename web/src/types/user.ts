export type User = {
  id: string;
  firstName: string;
  lastName: string;
  email: string;
  created_at: string;
  updated_at: string;
};

export type RegisterPayload = {
  firstName: string;
  lastName: string;
  email: string;
  password: string;
};
