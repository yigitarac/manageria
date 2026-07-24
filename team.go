package main

import "math/rand"

func TakimGucuHesapla(hesaplanacakTakim takim) takim {
	var toplamDefansGucu int
	var toplamOrtaSahaGucu int
	var toplamHucumGucu int
	var defansOyuncuSayisi int
	var ortaSahaOyuncuSayisi int
	var hucumOyuncuSayisi int
	for i := range hesaplanacakTakim.Kadro {
		if hesaplanacakTakim.Kadro[i].Mevki == "Kaleci" {
			hesaplanacakTakim.Kaleci = (rand.Intn((hesaplanacakTakim.Kadro[i].Yetenek)/5) + 4)
		}
		if hesaplanacakTakim.Kadro[i].Mevki == "Stoper" || hesaplanacakTakim.Kadro[i].Mevki == "SolBek" || hesaplanacakTakim.Kadro[i].Mevki == "SagBek" {
			toplamDefansGucu += hesaplanacakTakim.Kadro[i].Yetenek
			defansOyuncuSayisi++
		}
		if hesaplanacakTakim.Kadro[i].Mevki == "OrtaSaha" {
			toplamOrtaSahaGucu += hesaplanacakTakim.Kadro[i].Yetenek
			ortaSahaOyuncuSayisi++
		}
		if hesaplanacakTakim.Kadro[i].Mevki == "Forvet" || hesaplanacakTakim.Kadro[i].Mevki == "SolKanat" || hesaplanacakTakim.Kadro[i].Mevki == "SagKanat" {
			toplamHucumGucu += hesaplanacakTakim.Kadro[i].Yetenek
			hucumOyuncuSayisi++
		}
	}
	hesaplanacakTakim.Defans = toplamDefansGucu / defansOyuncuSayisi
	hesaplanacakTakim.OrtaSaha = toplamOrtaSahaGucu / ortaSahaOyuncuSayisi
	hesaplanacakTakim.Hucum = toplamHucumGucu / hucumOyuncuSayisi
	hesaplanacakTakim.OrtalamaGuc = (hesaplanacakTakim.Defans + hesaplanacakTakim.OrtaSaha + hesaplanacakTakim.Hucum) / 3

	return hesaplanacakTakim
}
