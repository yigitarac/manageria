package main

import "math/rand"

type MacDurumu struct {
	EvSahibiTakim        *takim
	DeplasmanTakimi      *takim
	Dakika               int
	AnlikBolge           int
	MacBittiMi           bool
	TopKimde             string
	TopaSahipOyuncuIndex int
	MacRaporu            []Olay
}

func YeniMacOlustur(ilkTakim *takim, ikinciTakim *takim) (baslangic MacDurumu) {
	baslangic = MacDurumu{
		Dakika:               0,
		AnlikBolge:           2,
		MacBittiMi:           false,
		TopaSahipOyuncuIndex: 6,
		EvSahibiTakim:        ilkTakim,
		DeplasmanTakimi:      ikinciTakim,
	}
	topKimdeZari := rand.Intn(2)
	if topKimdeZari == 0 {
		baslangic.TopKimde = "EvSahibi"
	} else {
		baslangic.TopKimde = "Deplasman"
	}
	return baslangic
}
