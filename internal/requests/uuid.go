package requests

import "github.com/jackc/pgx/v5/pgtype"

func uuidValue(id string) pgtype.UUID { var value pgtype.UUID; _ = value.Scan(id); return value }
