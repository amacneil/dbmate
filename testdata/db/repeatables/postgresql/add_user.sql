-- migrate:repeatable
CREATE OR REPLACE FUNCTION add_user (integer, text) RETURNS void AS $$
    insert into users (id, name) values ($1, $2);
$$ LANGUAGE SQL;
