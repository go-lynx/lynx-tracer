package tracer

import "github.com/go-lynx/lynx"

func currentLynxApp() *lynx.LynxApp {
	return lynx.Lynx()
}

func currentLynxName() string {
	return lynx.GetName()
}

func currentLynxHost() string {
	return lynx.GetHost()
}

func currentLynxVersion() string {
	return lynx.GetVersion()
}
