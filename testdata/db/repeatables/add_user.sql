-- migrate:repeatable
CREATE OR REPLACE FUNCTION add_user (integer, text)
RETURNS integer AS $total$
DECLARE
	total integer;
BEGIN
    insert into users (id, name) values ($1, $2);
    select count(id) into total from users;
    return total;
END;
$total$ LANGUAGE plpgsql;
