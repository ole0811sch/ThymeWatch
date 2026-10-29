package main

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/gen2brain/malgo"
	"github.com/tosone/minimp3"
	"org.thymewatch.daemon/ctxlog"
)

var maContext *malgo.AllocatedContext

func InitAudio() error {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(message string) {
		ctxlog.Printf(context.Background(), "[malgo] %v", message)
	})
	if err != nil {
		return err
	}
	maContext = ctx
	return nil
}

// onStop may be nil
func LoopMp3(mp3Path []string, onStop func()) (*malgo.Device, error) {
	if maContext == nil {
		err := InitAudio()
		if err != nil {
			return nil, err
		}
	}
	file, err := os.ReadFile(path.Join(mp3Path...))
	if err != nil {
		return nil, err
	}

	dec, data, err := minimp3.DecodeFull(file)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("Could not decode audio file (len(data) == 0)")
	}

	channels := uint32(dec.Channels)
	sampleRate := uint32(dec.SampleRate)
	dec.Close()

	deviceConfig := malgo.DefaultDeviceConfig(malgo.Playback)
	deviceConfig.Playback.Format = malgo.FormatS16
	deviceConfig.Playback.Channels = channels
	deviceConfig.SampleRate = sampleRate
	deviceConfig.Alsa.NoMMap = 1

	i := 0
	// This is the function that's used for sending more data to the device for playback.
	onSamples := func(pOutputSample, pInputSamples []byte, framecount uint32) {
		for j := 0; j < len(pOutputSample); j++ {
			pOutputSample[j] = data[i]
			i = (i + 1) % len(data)
		}
	}

	deviceCallbacks := malgo.DeviceCallbacks{
		Data: onSamples,
		Stop: onStop,
	}
	device, err := malgo.InitDevice(maContext.Context, deviceConfig, deviceCallbacks)
	if err != nil {
		return nil, err
	}

	err = device.Start()
	if err != nil {
		return nil, err
	}

	return device, nil
}

type SharedAlarmSoundContent struct {
	path   []string
	device *malgo.Device
}
type SharedAlarmSound struct {
	lb Lockbox[SharedAlarmSoundContent]
}

func MakeShareAlarmSound(mp3Path []string) SharedAlarmSound {
	return SharedAlarmSound{
		lb: MakeLockbox(SharedAlarmSoundContent{path: mp3Path}),
	}
}

func (sas *SharedAlarmSound) Play(ctx context.Context) error {
	err, err2 := sas.lb.WriteReturning(ctx, func(value *SharedAlarmSoundContent) error {
		if value.device != nil && value.device.IsStarted() {
			return nil
		}

		if value.device != nil {
			value.device.Uninit()
			value.device = nil
		}
		dev, err := LoopMp3(value.path, nil)
		if err != nil {
			return err
		}
		value.device = dev
		return nil
	})
	if err2 != nil {
		return err2
	}
	return err
}

func (sas *SharedAlarmSound) Stop(ctx context.Context) error {
	err, err2 := sas.lb.WriteReturning(ctx, func(value *SharedAlarmSoundContent) error {
		if value.device == nil {
			return nil
		}
		if value.device.IsStarted() {
			err := value.device.Stop()
			if err != nil {
				return err
			}
		}
		value.device = nil
		return nil
	})
	if err2 != nil {
		return err2
	}
	return err
}
