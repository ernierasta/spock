package config

import (
	"fmt"
	"time"

	"github.com/ernierasta/zorix/shared"
	"github.com/ernierasta/zorix/template"

	"github.com/BurntSushi/toml"
)

const (
	Loglevel    = "warn"
	HTTPTimeout = "60s"
	PingTimeout = "60s"
	PortTimeout = "5s"

	CheckType         = "web"
	CheckMethod       = "GET"
	CheckRepeat       = "60s"
	CheckExpectedCode = 200
	CheckExpectedTime = 1000
	CheckAllowedSlows = 3
	CheckAllowedFails = 1

	NotifyType          = "mail"
	NotifySubjectFail   = "{check}{params} problem"
	NotifySubjectSlow   = "{check}{params} slow"
	NotifySubjectFailOK = "{check}{params} ok"
	NotifySubjectSlowOK = "{check}{params} ok"
	NotifyTextFail      = "FAILURE:\n{check}{params}\nTime: {timestamp}\n\nResponse code: {response_code}\nError: {error}\n"
	NotifyTextSlow      = "SLOW RESPONSE:\n{check}{params}\nTime: {timestamp}\n\nResponse/Expected time: {response_time}/{expected_time}\n"
	NotifyTextFailOK    = "RECOVERED:\n{check}{params}\nTime: {timestamp}\n\nResponse code: {response_code}\n"
	NotifyTextSlowOK    = "RECOVERED:\n{check}{params}\nTime: {timestamp}\n\nResponse/Expected time: {response_time}/{expected_time}\n"
)

var (
	// notifTypes is slice of available notifications. Empty is also ok, will be normalized.
	// Add new type here!
	notifTypes = []string{"", "mail", "jabber", "cmd"}
)

// Config represents whole configuration file parsed to stuct.
type Config struct {
	Global        shared.Global
	Notifications []shared.NotifConfig `toml:"notify"`
	Checks        []shared.CheckConfig `toml:"check"`
	file          string
}

func New(file string) *Config {
	return &Config{file: file}
}

func (c *Config) Read() error {

	_, err := toml.DecodeFile(c.file, c)
	if err != nil {
		return fmt.Errorf("in file %s: err: %s", c.file, err)
	}
	return nil

}

// Validate will check if all necessary fields are given
func (c *Config) Validate() error {
	if err := c.validateGlobal(); err != nil {
		return err
	}
	if err := c.validateChecks(); err != nil {
		return err
	}
	return c.validateNotifications()
}

func (c *Config) validateGlobal() error {
	if c.Global.Workers == 0 {
		return fmt.Errorf("config.validate: [global] workers not defined (cur val: %d), fix config file", c.Global.Workers)
	}

	return nil
}

func (c *Config) validateChecks() error {
	if len(c.Checks) == 0 {
		return fmt.Errorf("config.validate: no checks defined, fix config file")
	}

	for i, check := range c.Checks {
		i++ // count from 1
		if check.ID == "" {
			return fmt.Errorf("config.validate: empty 'ID' in %d. check. This field is mandatory, fix config file", i)
		}
		if check.Check == "" {
			return fmt.Errorf("config.validate: empty 'check' for %q. check. This field is mandatory, fix config file", check.ID)
		}
		if check.NotifyFail != nil {
			if err := c.validateNotifyIDList(check.NotifyFail); err != nil {
				return fmt.Errorf("config.validate: wrong notification in 'notify_fail' for %q. check, err: %v. fix config file", check.ID, err)
			}
		}
		if check.NotifySlow != nil {
			if err := c.validateNotifyIDList(check.NotifySlow); err != nil {
				return fmt.Errorf("config.validate: wrong notification in 'notify_slow' for %q check, err: %v. fix config file", check.ID, err)
			}
		}
	}
	return nil
}

func (c *Config) validateNotifications() error {
	for i, notif := range c.Notifications {
		i++ //count from 1
		if notif.ID == "" {
			return fmt.Errorf("config.validate: empty 'ID' for %d. notification. This field is mandatory, fix config file", i)
		}
		if !found(notif.Type, notifTypes) {
			return fmt.Errorf("config.validate: unknown Type for %q notification. Check config file", notif.ID)
		}
		if notif.Type != "cmd" && notif.Server == "" {
			return fmt.Errorf("config.validate: empty 'server' for %q notification. This field is mandatory, fix config file", notif.ID)
		}
		if notif.Type != "cmd" && notif.Port == 0 {
			return fmt.Errorf("config.validate: Given 0 as 'port' for %q notification. This field must be non-zero, fix config file", notif.ID)
		}
		if notif.Type != "cmd" && notif.From == "" && notif.User == "" {
			return fmt.Errorf("config.validate: empty 'from' for %q notification. This field is mandatory, fix config file", notif.ID)
		}
		if notif.Type != "cmd" && notif.To == nil {
			return fmt.Errorf("config.validate: empty 'to' for %q notification. This field is mandatory, fix config file", notif.ID)
		}
		if notif.Type == "cmd" && notif.CmdTemplate == "" {
			return fmt.Errorf("config.validate: empty 'cmd' for %q notification. This field is mandatory, fix config file", notif.ID)
		}

	}
	return nil
}

// Normalize will fill in default values if missing in config
func (c *Config) Normalize() {
	c.normalizeGlobal()
	c.normalizeChecks()
	c.normalizeNotifications()

	c.parseCheckVars()
	c.parseNotifVars()
}

func (c *Config) normalizeGlobal() {
	if c.Global.Loglevel == "" {
		c.Global.Loglevel = Loglevel
	}
	if c.Global.HTTPTimeout.Duration == 0 {
		c.Global.HTTPTimeout.ParseDuration(HTTPTimeout)
	}
	if c.Global.PingTimeout.Duration == 0 {
		c.Global.PingTimeout.ParseDuration(PingTimeout)
	}
	if c.Global.PortTimeout.Duration == 0 {
		c.Global.PortTimeout.ParseDuration(PortTimeout)
	}
}

func (c *Config) normalizeChecks() {
	notifids := c.getAllNotificationIDs()
	for i, check := range c.Checks {
		if check.Type == "" {
			c.Checks[i].Type = CheckType
		}
		if check.Method == "" {
			c.Checks[i].Method = CheckMethod
		}
		if check.Repeat.Duration == 0 {
			c.Checks[i].Repeat.ParseDuration(CheckRepeat)
		}
		if check.ExpectedCode == 0 {
			c.Checks[i].ExpectedCode = CheckExpectedCode
		}
		if check.ExpectedTime == 0 {
			c.Checks[i].ExpectedTime = CheckExpectedTime
		}
		if check.AllowedFails < 1 {
			c.Checks[i].AllowedFails = CheckAllowedFails
		}
		if check.AllowedSlows < 1 {
			c.Checks[i].AllowedSlows = CheckAllowedSlows
		}
		if check.NotifyFail == nil {
			c.Checks[i].NotifyFail = notifids
		}
		if check.NotifySlow == nil {
			c.Checks[i].NotifySlow = notifids
		}
	}
}

// parseCheckVars will expand all $var or ${var} to actual
// enviroment variable.
func (c *Config) parseCheckVars() {
	for i, check := range c.Checks {
		c.Checks[i].Params = template.ParseEnv(check.Params, check.ID, "params")
		c.Checks[i].Headers = template.ParseEnv(check.Headers, check.ID, "headers")
	}

}

func (c *Config) parseNotifVars() {
	for i, notif := range c.Notifications {
		c.Notifications[i].User = template.ParseEnv(notif.User, notif.ID, "user")
		c.Notifications[i].Pass = template.ParseEnv(notif.Pass, notif.ID, "pass")
		c.Notifications[i].Server = template.ParseEnv(notif.Server, notif.ID, "server")
	}
}

// getAllNotificationIDs returns slice of all notification IDs.
func (c *Config) getAllNotificationIDs() []string {
	ids := []string{}
	for _, notif := range c.Notifications {
		ids = append(ids, notif.ID)
	}
	return ids
}

// validateNotifyIDList returns error if any notification on list
// is not found in defined notifications.
func (c *Config) validateNotifyIDList(ss []string) error {
	for _, nID := range ss {
		if !found(nID, c.getAllNotificationIDs()) {
			return fmt.Errorf("notification %q is not defined", nID)
		}
	}
	return nil
}

func (c *Config) normalizeNotifications() {
	for i, notif := range c.Notifications {
		if notif.Type == "" {
			c.Notifications[i].Type = NotifyType
		}
		if notif.From == "" {
			c.Notifications[i].From = notif.User
		}
		c.Notifications[i].SubjectFail = setTemplate(notif.SubjectFail, c.Global.NotifySubjectFail, NotifySubjectFail)
		c.Notifications[i].SubjectFailOK = setTemplate(notif.SubjectFailOK, c.Global.NotifySubjectFailOK, NotifySubjectFailOK)
		c.Notifications[i].TextFail = setTemplate(notif.TextFail, c.Global.NotifyTextFail, NotifyTextFail)
		c.Notifications[i].TextFailOK = setTemplate(notif.TextFailOK, c.Global.NotifyTextFailOK, NotifyTextFailOK)
		c.Notifications[i].SubjectSlow = setTemplate(notif.SubjectSlow, c.Global.NotifySubjectSlow, NotifySubjectSlow)
		c.Notifications[i].SubjectSlowOK = setTemplate(notif.SubjectSlowOK, c.Global.NotifySubjectSlowOK, NotifySubjectSlowOK)
		c.Notifications[i].TextSlow = setTemplate(notif.TextSlow, c.Global.NotifyTextSlow, NotifyTextSlow)
		c.Notifications[i].TextSlowOK = setTemplate(notif.TextSlowOK, c.Global.NotifyTextSlowOK, NotifyTextSlowOK)

		if notif.RepeatFail == nil {
			c.Notifications[i].RepeatFail = []shared.Duration{
				shared.Duration{Duration: 1 * time.Minute},
				shared.Duration{Duration: 5 * time.Minute},
				shared.Duration{Duration: 10 * time.Minute},
			}
		}
		if notif.RepeatSlow == nil {
			c.Notifications[i].RepeatSlow = []shared.Duration{
				shared.Duration{Duration: 5 * time.Minute},
				shared.Duration{Duration: 0},
			}
		}
	}
}

func setTemplate(t, glob, def string) string {
	if t == "" {
		if glob != "" {
			return glob
		}
		return def
	}
	return t
}

func found(s string, ss []string) bool {
	found := false
	for _, t := range ss {
		if s == t {
			found = true
		}
	}
	return found
}
