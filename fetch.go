package main

import (
	"context"
	"os"
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

func startCron() {

	if os.Getenv("PURE_USER") != "" && os.Getenv("PURE_PASS") != "" {
		c := cron.New()
		_, err := c.AddFunc("@every 10m", trigger)
		if err != nil {
			logger.Error("adding cron", zap.Error(err))
			return
		}
		c.Start()
	}
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
	if len(members) == 3 {
		now, err := strconv.Atoi(strings.Replace(members[1], ",", "", 1))
		if err != nil {
			logger.Error("parsing members", zap.Error(err))
			return
		}

		max, err := strconv.Atoi(strings.Replace(members[2], ",", "", 1))
		if err != nil {
			logger.Error("parsing members", zap.Error(err))
			return
		}

		pct := float64(now) / float64(max)

		logger.Info("members", zap.Int("now", now), zap.Int("max", max), zap.Float64("pct", pct), zap.String("town", town))
	}
}

func loginAndCheckMembers(ctx context.Context) (people, town string, err error) {

	actions := []chromedp.Action{
		network.Enable(),
		chromedp.ActionFunc(func(ctx context.Context) error {

			logger.Info("Setting cookies", zap.Int("count", len(cookies)))

			for _, v := range cookies {
				expr := cdp.TimeSinceEpoch(time.Unix(int64(v.Expires), 0))
				_, err := network.SetCookie(v.Name, v.Value).
					WithExpires(&expr).
					WithDomain(v.Domain).
					WithHTTPOnly(v.HTTPOnly).
					WithPath(v.Path).
					WithPriority(v.Priority).
					WithSameSite(v.SameSite).
					WithSecure(v.Secure).
					Do(ctx)

				if err != nil {
					return err
				}
			}

			return nil
		}),
		chromedp.Emulate(device.IPadPro),
		chromedp.Navigate("https://www.puregym.com/members/"),
		chromedp.WaitVisible("#loginForm, #people_in_gym"),
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
			err = chromedp.Nodes("#loginForm", &loginNodes, chromedp.AtLeast(0)).Do(ctx)
			if err != nil {
				return err
			}

			if len(loginNodes) > 0 {

				logger.Info("Logging in")

				err = chromedp.SendKeys("#loginForm input[type=email]", os.Getenv("PURE_USER")).Do(ctx)
				if err != nil {
					return err
				}

				err = chromedp.SendKeys("#loginForm input[type=password]", os.Getenv("PURE_PASS")).Do(ctx)
				if err != nil {
					return err
				}

				err = chromedp.Click("#login-submit", chromedp.ByID).Do(ctx)
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
