package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/sovereignite/shuttle/internal/phonehome"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("distaff", flag.ContinueOnError)
	instanceID := flags.String("instance-id", "", "cloud instance ID")
	port := flags.Uint("port", 0, "host vsock port")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *instanceID == "" || *port == 0 || uint64(*port) > uint64(^uint32(0)) {
		return fmt.Errorf("usage: distaff --instance-id ID --port PORT")
	}
	payload, err := phonehome.Collect(*instanceID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	return phonehome.Send(ctx, uint32(*port), payload)
}
