package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/device"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

var (
	membersRegex = regexp.MustCompile(`(?i)([0-9,]{1,4})\s(of)?\s?([0-9,]{1,4})?`)
	cookies      []*network.Cookie
	logger       *zap.Logger
)

func main() {

	logger, _ = zap.NewDevelopment()

	defer func() {
		err := logger.Sync()
		if err != nil {
			fmt.Println(err)
		}
	}()

	if os.Getenv("PURE_USER") != "" && os.Getenv("PURE_PASS") != "" {

		trigger()

		c := cron.New()
		_, err := c.AddFunc("*/10 * * * *", trigger)
		if err != nil {
			logger.Error("adding cron", zap.Error(err))
			return
		}
		c.Start()
	}

	webserver()
}

func trigger() {

	ctx := context.Background()

	ctx, cancel1 := chromedp.NewContext(ctx)
	defer cancel1()

	ctx, cancel2 := context.WithTimeout(ctx, 30*time.Second)
	defer cancel2()

	peopleString, town, err := loginAndCheckMembers(ctx)
	if err != nil {
		logger.Error("running chromedp", zap.Error(err))
		return
	}

	members := membersRegex.FindStringSubmatch(peopleString)
	if len(members) == 4 {

		now, err := strconv.Atoi(strings.Replace(members[1], ",", "", 1))
		if err != nil {
			logger.Error("parsing members", zap.Error(err))
			return
		}

		max, err := strconv.Atoi(strings.Replace(members[3], ",", "", 1))
		if err != nil {
			logger.Error("parsing members", zap.Error(err))
			return
		}

		pct := float64(now) / float64(max)

		logger.Info("members", zap.Int("now", now), zap.Int("max", max), zap.Float64("pct", pct), zap.String("town", town))

		f, err := os.OpenFile("counts.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			logger.Error("open a file", zap.Error(err))
		}

		defer f.Close()

		if _, err := f.WriteString(strconv.FormatInt(time.Now().Unix(), 10) + " " + strconv.Itoa(now) + "\n"); err != nil {
			logger.Error("appending to file", zap.Error(err))
		}
	}
}

func loginAndCheckMembers(ctx context.Context) (people, town string, err error) {

	actions := []chromedp.Action{
		network.Enable(),
		chromedp.ActionFunc(func(ctx context.Context) error {

			if len(cookies) > 0 {

				logger.Info("Setting cookies", zap.Int("count", len(cookies)))

				for _, cookie := range cookies {

					expr := cdp.TimeSinceEpoch(time.Unix(int64(cookie.Expires), 0))
					err := network.SetCookie(cookie.Name, cookie.Value).
						WithExpires(&expr).
						WithDomain(cookie.Domain).
						WithHTTPOnly(cookie.HTTPOnly).
						WithPath(cookie.Path).
						WithPriority(cookie.Priority).
						WithSameSite(cookie.SameSite).
						WithSecure(cookie.Secure).
						Do(ctx)

					if err != nil {
						return err
					}
				}
			}

			return nil
		}),
		chromedp.Emulate(device.IPadPro),
		chromedp.Navigate("https://www.puregym.com/members/"),
		chromedp.WaitVisible("input[name=username], input[name=password], #people_in_gym"),
		chromedp.ActionFunc(func(ctx context.Context) error {

			// Accept cookies, probably don't need to bother
			var cookieNodes []*cdp.Node
			err = chromedp.Nodes("button.coi-banner__accept", &cookieNodes, chromedp.AtLeast(0)).Do(ctx)
			if err != nil {
				return err
			}

			if len(cookieNodes) > 0 {

				logger.Info("Submitting cookie popup")
				_, exp, err := runtime.Evaluate("CookieInformation.submitAllCategories();").Do(ctx)
				if err != nil {
					return err
				}
				if exp != nil {
					return exp
				}
			}

			// Login
			var loginNodes []*cdp.Node
			err = chromedp.Nodes("input[name=username], input[name=password]", &loginNodes, chromedp.AtLeast(0)).Do(ctx)
			if err != nil {
				return err
			}

			if len(loginNodes) > 0 {

				logger.Info("Logging in")

				err = chromedp.SendKeys("input[name=username]", os.Getenv("PURE_USER")).Do(ctx)
				if err != nil {
					return err
				}

				err = chromedp.SendKeys("input[name=password]", os.Getenv("PURE_PASS")).Do(ctx)
				if err != nil {
					return err
				}

				err = chromedp.Click("button[value=login]", chromedp.ByQuery).Do(ctx)
				if err != nil {
					return err
				}
			}

			return nil
		}),
		chromedp.WaitVisible("#people_in_gym"),
		chromedp.ActionFunc(func(ctx context.Context) error {

			logger.Info("Logged in, taking cookies")

			var err error
			cookies, err = network.GetAllCookies().Do(ctx)
			return err
		}),
		chromedp.InnerHTML("#people_in_gym span", &people),
		chromedp.InnerHTML("#people_in_gym a", &town),
	}

	err = chromedp.Run(ctx, actions...)
	return people, town, err
}
