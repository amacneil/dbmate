-- migrate:up
create table posts (
  id int64,
  name string
);

-- migrate:down
drop table posts;
