-- migrate:up
create table categories (
  id integer,
  title varchar(50),
  slug varchar(100)
);

-- migrate:down
drop table categories;
