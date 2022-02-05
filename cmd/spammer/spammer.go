package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/urfave/cli/v2"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func readDiscard(r io.Reader) {
	buf := make([]byte, 2048)
	for {
		_, err := r.Read(buf[:])
		if err != nil {
			break // end reader process -- this err should occur as we are disconnected...
		} else {
			// fmt.Printf("GOT '%s'\n", buf[0:n])
		}
		// do nothing with read output
	}
}

type SpamWorkerResult struct {
	ID          int
	NumMessages int
}

func spamWorker(protocol string, endpoint string, id int, waitDuration time.Duration, ctx context.Context, resultCh chan<- SpamWorkerResult) error {
	// construct a message of format [<header>, <data>]
	// where header [<length>, <op-code>] and data is 'hello, friend'
	msg := make([]byte, 22)
	binary.LittleEndian.PutUint32(msg, 14)
	binary.LittleEndian.PutUint32(msg[4:], 1)
	copy(msg[8:], "hello, friend\n")

	conn, err := net.Dial(protocol, endpoint)
	if err != nil {
		log.Fatal("connection error: ", err)
	}
	defer conn.Close()   // TODO: unhandled error, how to handle...?
	go readDiscard(conn) // TODO: parametrize

	res := SpamWorkerResult{ID: id, NumMessages: 0}
	for {
		select {
		case <-ctx.Done():
			// we are done, inform caller of why we are done...
			resultCh <- res
			return ctx.Err()
		default:
			err := conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if err != nil {
				panic("failed to enforce write deadline on connection")
			}
			_, err = conn.Write(msg)
			if err, ok := err.(net.Error); ok && err.Timeout() {
			} else if err != nil {
				log.Fatal("client write error: ", err)
			} else {
				res.NumMessages += 1
			}
			if waitDuration != 0 {
				time.Sleep(waitDuration)
			}
		}
	}
}

func spam(protocol string, endpoint string, delay int, numjobs int, seconds int) error {
	fmt.Printf("Job Params\n===========\n\tJobs:  %d\n\tDuration:  %ds\n\tDelay:  %dmsecs\n\tProtocol:  %s\n\tEndpoint:  %s\n", numjobs, seconds, delay, protocol, endpoint)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(seconds)*time.Second)
	defer cancel()

	signals := make(chan os.Signal)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signals
		fmt.Println("Aborted")
		cancel()
	}()
	defer close(signals)

	resultCh := make(chan SpamWorkerResult, numjobs)
	errs := make(chan error, numjobs+1)
	defer close(errs)

	wait := &sync.WaitGroup{}
	wait.Add(numjobs)
	for i := 1; i <= numjobs; i++ {
		i := i
		go func() {
			err := spamWorker(protocol, endpoint, i, time.Duration(delay)*time.Millisecond, ctx, resultCh)
			if err != nil {
				cancel() // cancel all work (other workers too)
				if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					// genuine error, report back
					errs <- err
				}
			}
			wait.Done()
		}()
	}

	fmt.Println("waiting for workers to quit...")
	wait.Wait()
	resultsReceived := 0
	totalMsgs := 0

	// wait until all workers are done, close results channel to prevent additional messages
	wait.Wait()
	close(resultCh)

	select {
	case err := <-errs:
		fmt.Println("something went wrong :(")
		return err
	default:
	}

	for res := range resultCh {
		totalMsgs += res.NumMessages
		resultsReceived += 1
	}

	fmt.Printf("\nResults\n==========\nJobs: %d\nAvg msgs/sec: %f\n\nAggregate:\n\tMsgs: %d\n\tMsgs/sec: %f\n",
		numjobs,
		float64(totalMsgs)/float64(seconds)/float64(numjobs),
		totalMsgs,
		float64(totalMsgs)/float64(seconds))
	return nil
}

func main() {
	var numjobs int
	var seconds int
	var endpoint string
	var delay int
	var proto string

	app := &cli.App{
		Name:  "spammer",
		Usage: "post all the things",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "endpoint",
				Aliases:     []string{"e"},
				Value:       "/tmp/test.sock",
				Destination: &endpoint,
				Usage:       "host:port or path to the unix socket",
			},
			&cli.StringFlag{
				Name:        "protocol",
				Aliases:     []string{"p"},
				Value:       "tcp",
				Destination: &proto,
				Usage:       "protocol to use (tcp, udp)",
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "spam",
				Usage: "run spam test",
				Action: func(c *cli.Context) error {
					return spam(proto, endpoint, delay, numjobs, seconds)
				},
				Flags: []cli.Flag{
					&cli.IntFlag{
						Name:        "jobs",
						Aliases:     []string{"j"},
						Value:       3,
						Destination: &numjobs,
						Usage:       "number of concurrent client jobs",
					},
					&cli.IntFlag{
						Name:        "seconds",
						Aliases:     []string{"s"},
						Value:       5,
						Destination: &seconds,
						Usage:       "number of seconds for which to run the test.",
					},
					&cli.IntFlag{
						Name:        "delay",
						Aliases:     []string{"d"},
						Value:       0,
						Destination: &delay,
						Usage:       "number of msecs to wait between sending messages",
					},
				},
			},
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}
