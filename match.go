package main

import "math/rand"

const evSahibiSabiti = "EvSahibi"
const deplasmanSabiti = "Deplasman"

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

func (r MacDurumu) BaslangicOyuncusu() (oyuncu Futbolcu) {

	if r.TopKimde == evSahibiSabiti {
		oyuncu = r.EvSahibiTakim.Kadro[r.TopaSahipOyuncuIndex]
	} else {
		oyuncu = r.DeplasmanTakimi.Kadro[r.TopaSahipOyuncuIndex]
	}

	return oyuncu
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
		baslangic.TopKimde = evSahibiSabiti
	} else {
		baslangic.TopKimde = deplasmanSabiti
	}
	return baslangic
}
