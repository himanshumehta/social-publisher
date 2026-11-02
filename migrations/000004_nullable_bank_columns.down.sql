-- Rollback: Restore NOT NULL constraints on bank account columns
-- Note: This will fail if any rows have NULL values in these columns.

UPDATE banking_info SET account_holder = '' WHERE account_holder IS NULL;
UPDATE banking_info SET bank_name = '' WHERE bank_name IS NULL;
UPDATE banking_info SET account_number = '' WHERE account_number IS NULL;
UPDATE banking_info SET ifsc_code = '' WHERE ifsc_code IS NULL;

ALTER TABLE banking_info ALTER COLUMN account_holder SET NOT NULL;
ALTER TABLE banking_info ALTER COLUMN bank_name SET NOT NULL;
ALTER TABLE banking_info ALTER COLUMN account_number SET NOT NULL;
ALTER TABLE banking_info ALTER COLUMN ifsc_code SET NOT NULL;
