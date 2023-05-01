-- migrate:up env-var:YABBA_DABBA_DOO
create table posts (
  id integer,
  name varchar(255)
);
insert into posts (id, name) values (1, '{{ .YABBA_DABBA_DOO }}');

-- migrate:down
drop table posts;
