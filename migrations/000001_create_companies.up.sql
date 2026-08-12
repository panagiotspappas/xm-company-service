CREATE TABLE companies (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    amount_of_employees INTEGER NOT NULL,
    registered BOOLEAN NOT NULL,
    type TEXT NOT NULL,

    CONSTRAINT companies_name_unique
        UNIQUE (name),
    CONSTRAINT companies_name_not_blank
        CHECK (btrim(name) <> ''),
    CONSTRAINT companies_name_length
        CHECK (char_length(name) <= 15),
    CONSTRAINT companies_description_length
        CHECK (char_length(description) <= 3000),
    CONSTRAINT companies_employee_count_non_negative
        CHECK (amount_of_employees >= 0),
    CONSTRAINT companies_type_valid
        CHECK (
            type IN (
                'Corporations',
                'NonProfit',
                'Cooperative',
                'Sole Proprietorship'
            )
        )
);
