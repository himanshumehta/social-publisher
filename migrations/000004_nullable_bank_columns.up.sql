-- Migration: Make bank account columns nullable to support UPI-only banking
-- Previously account_holder, bank_name, account_number, ifsc_code were NOT NULL,
-- which prevented UPI-only banking submissions from working correctly.

ALTER TABLE banking_info ALTER COLUMN account_holder DROP NOT NULL;
ALTER TABLE banking_info ALTER COLUMN bank_name DROP NOT NULL;
ALTER TABLE banking_info ALTER COLUMN account_number DROP NOT NULL;
ALTER TABLE banking_info ALTER COLUMN ifsc_code DROP NOT NULL;
