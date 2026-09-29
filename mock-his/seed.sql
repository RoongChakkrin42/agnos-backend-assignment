-- docker compose demo only (not used by tests): point both hospitals at the mock-his container
-- instead of the real HIS domains from the assignment, which don't exist.
UPDATE hospitals SET api_base_url = 'http://mock-his/' || code;
