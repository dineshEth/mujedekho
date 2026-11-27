package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
)

func main() {
	ctx := context.Background()
 conn, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Println("connect:", err)
		os.Exit(1)
	}
	defer conn.Close(ctx)
	var col string
	rows, err := conn.Query(ctx, `SELECT column_name FROM information_schema.columns WHERE table_name = 'monitoring_checks' ORDER BY ordinal_position`)
	if err != nil {
		fmt.Println("query:", err)
		os.Exit(1)
	}
	for rows.Next() {
		_ = rows.Scan(&col)
		fmt.Print(col, " ")
	}
	fmt.Println()
}
