// SPDX-License-Identifier: MIT
// Copyright (c) 2026 eshu-hq

// Command collector is the Eshu Prometheus-Mimir collector entrypoint. With
// no arguments (or --sdk-stdio) it serves one claim: it reads one
// collector-sdk/v1alpha1 request from stdin and writes one result to stdout.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	collector "github.com/eshu-hq/eshu-collector-prometheus-mimir"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv); err != nil {
		fmt.Fprintf(os.Stderr, "collector: %v\n", err)
		os.Exit(1)
	}
}

func run(
	ctx context.Context,
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	getenv func(string) string,
) error {
	return runClaim(ctx, args, stdin, stdout, stderr, getenv, time.Now)
}

func runClaim(
	ctx context.Context,
	args []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	getenv func(string) string,
	now func() time.Time,
) error {
	flags := flag.NewFlagSet("collector", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Bool("sdk-stdio", true, "read one collector SDK host request from stdin")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", flags.Arg(0))
	}
	request, err := collector.ParseRequest(stdin)
	if err != nil {
		return err
	}
	handler := collector.Handler{Getenv: getenv, Now: now, Log: stderr}
	result := handler.Handle(ctx, request)
	encoded, err := json.Marshal(result)
	if err != nil {
		return errors.New("encode result")
	}
	if _, err := stdout.Write(append(encoded, '\n')); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	return nil
}
