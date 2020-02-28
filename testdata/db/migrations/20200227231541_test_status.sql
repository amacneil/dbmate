-- migrate:up
create table foo (
  id integer,
  name varchar(255)
);

-- migrate:down
drop table foo;
