-- migrate:up
create table posts (
  id integer,
  name varchar(255)
);

-- migrate:down
drop table posts;
