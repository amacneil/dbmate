-- migrate:up
create table users (
  id int64,
  name string
);
insert into users (id, name) values (1, 'alice');

-- migrate:down
drop table users;
