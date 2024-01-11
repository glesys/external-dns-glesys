/*
Copyright 2023 GleSYS AB

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"github.com/glesys/external-dns-glesys/dnsprovider"
	"github.com/glesys/external-dns-glesys/webhook"
	"github.com/glesys/external-dns-glesys/webhook/configuration"
	"github.com/glesys/external-dns-glesys/webhook/logging"
	"github.com/glesys/external-dns-glesys/webhook/server"
	log "github.com/sirupsen/logrus"
)

const banner = `
  ________.____     ___________  _____________.___. _________
 /  _____/|    |    \_   _____/ /  _____/\__  |   |/   _____/
/   \  ___|    |     |    __)_  \____  \  /   |   |\_____  \
\    \_\  \    |___  |        \/        \ \____   |/        \
 \______  /_______ \/_______  /_______  / / ______/_______  /
        \/        \/        \/        \/  \/              \/

 external-dns-glesys
 version: %s

`

var (
	Version = "v0.0.3"
)

func main() {
	fmt.Printf(banner, Version)
	logging.Init()
	config := configuration.Init()

	provider, err := dnsprovider.NewGlesysProvider(false, Version)
	if err != nil {
		log.Fatalf("Failed to initialize DNS provider: %v", err)
	}
	srv := server.Init(config, webhook.New(provider))
	server.ShutdownGracefully(srv)
}
