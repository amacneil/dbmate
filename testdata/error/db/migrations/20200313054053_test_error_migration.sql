-- migrate:up
create table test (
  id integer
);
create table test_mistake (
  -- uh oh, comma here on the last field, error!
  id integer,
);

-- migrate:down
drop table test;
drop table test_mistake;
