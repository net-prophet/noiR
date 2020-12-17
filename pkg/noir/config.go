package noir

import (
	log "github.com/pion/ion-log"
	"github.com/pion/ion-sfu/pkg/sfu"
)

type Config struct {
	Ion sfu.Config
	Log log.Config `mapstructure:"log"`
}
