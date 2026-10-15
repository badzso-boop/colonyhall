-- A separate database for the Postgres store tests (`make test-pg`), so
-- they never wipe the development data.
CREATE DATABASE colony_test OWNER colony;
