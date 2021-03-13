-- migrate:up
create table users (
  id integer,
  name varchar(255)
);
insert into users (id, name) values (1, 'alice');

-- migrate:down
drop table users;
