-- migrate:up env:YABBA_DABBA_DOO
create table posts (
  id integer,
  name varchar(255)
);
insert into posts (id, name) values (1, '{{ .YABBA_DABBA_DOO }}');
insert into posts (id, name) values (2, '{{ or (index . "MISSING_DINO") "Dino" }}');

-- migrate:down
drop table posts;
