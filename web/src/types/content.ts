export type Tag = {
  id: string;
  name: string;
  color: string;
};

export type Piece = {
  id: string;
  title: string;
  content: string;
  tag_ids: string[];
  tags: Tag[];
  created_by: string;
  created_at: string;
  updated_at: string;
};

export type CreatePiecePayload = {
  title: string;
  content: string;
  tag_ids?: string[];
};

export type UpdatePiecePayload = {
  title: string;
  content: string;
  tag_ids?: string[];
};
