-- +goose Up
CREATE TABLE chirps(
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL,
  body TEXT NOT NULL,
  constraint fk_user_id
  FOREIGN KEY (user_id)
  REFERENCES users(id) ON DELETE CASCADE
);




-- +goose Down
DROP TABLE chirps;
