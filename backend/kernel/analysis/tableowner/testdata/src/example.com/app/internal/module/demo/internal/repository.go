package internal

const mine = `SELECT id FROM demo_thing WHERE id = $1`

const theirs = `SELECT a.name FROM demo_thing d JOIN other_account a ON a.id = d.account_id` // want `other_account is the other module's table - reach it through other/contract`

const write = "UPDATE other_account SET name = $1" // want `other_account is the other module's table`

// The kernel's tables belong to no module.
const kernel = `INSERT INTO audit_entry (action) VALUES ($1)`

// A word that only looks like a table: EXTRACT(EPOCH FROM now()).
const epoch = `SELECT EXTRACT(EPOCH FROM now())`
