package main

import (
	"os"
	"time"

	log "github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"

	"github.com/ernierasta/zorix/check"
	"github.com/ernierasta/zorix/config"
	"github.com/ernierasta/zorix/logger"
	"github.com/ernierasta/zorix/notify"
	"github.com/ernierasta/zorix/processor"
	"github.com/ernierasta/zorix/shared"
)

var (
	conf  string
	logf  string
	testf bool
)

func init() {
	flag.StringVarP(&conf, "config", "c", "config.toml", "config file location")
	flag.StringVarP(&logf, "log", "l", "", "if you want to log , point it to file")
	flag.BoolVarP(&testf, "test", "t", false, "test notification configuration(cmd writes file /tmp/zorix.test)")
	flag.Parse()
}

func main() {

	// temporary set logging, just to catch config file errors
	logger.Set(logf, "warn")
	c := config.New(conf)
	err := c.Read()
	if err != nil {
		log.Fatal(err)
	}

	logger.Set(logf, c.Global.Loglevel)
	log.Warn("Config loaded ok. Starting ...")

	err = c.Validate()
	if err != nil {
		log.Fatal(err)
	}
	c.Normalize()

	//all results goes there
	resultsChan := make(chan shared.CheckConfig, len(c.Checks)*10)
	notifChan := make(chan shared.NotifiedCheck, len(c.Checks)*10)

	cc := shared.CMConfig{
		Checks:      c.Checks,
		Workers:     c.Global.Workers,
		ResultsChan: resultsChan,
		HTTPTimeout: c.Global.HTTPTimeout,
		PingTimeout: c.Global.PingTimeout,
		PortTimeout: c.Global.PortTimeout,
	}

	chm := check.NewManager(cc)
	proc := processor.New(resultsChan, notifChan, len(c.Checks), c.Notifications)
	nm := notify.NewManager(notifChan, c.Notifications)

	if testf {
		nm.TestAll()
		os.Exit(0)
	}

	// start listening for results
	proc.Listen()
	// start listening for notifications
	nm.Listen()
	// run checks
	chm.Register()
	chm.Run()

	log.Warn("All ok. Checks are runing.")
	for {
		time.Sleep(1 * time.Second)
	}
}
