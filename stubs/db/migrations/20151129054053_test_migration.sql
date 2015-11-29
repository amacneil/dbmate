-- migrate:up
CREATE TABLE users (
  id integer,
  name varchar
);
INSERT INTO users (id, name) VALUES (1, 'alice');

-- migrate:down
DROP TABLE users;
