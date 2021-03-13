-- migrate:up
CREATE TABLE test (
    test_id uuid NOT NULL
);
COMMENT ON TABLE test IS 'Here is my test table';

CREATE TABLE test_mistake (
    -- uh oh, comma here on the last field, error!
    test_mistake_id uuid NOT NULL,
);

-- migrate:down
DROP TABLE test;
DROP TABLE test_mistake;
