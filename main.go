package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
)

func main() {

	/*kacanMesajlar := []string{"TOP DİREKTE PATLADI", "KALE AĞZINDAN DIŞARIYA VURDU", "BUNU NASIL KAÇIRIR?", "TAKIM ARKADAŞLARINDAN ÖZÜR DİLİYOR", "TOP DİREĞİN YANINDAN DIŞARIYA GİDİYOR", "AZ FARKLA AUT", "DAĞLARA TAŞLARA", "REZİL BİR ŞUT"}
	asistMesajlari := []string{"AKIL DOLU BİR PAS", "MÜTHİŞ SERVİS", "TEKTE OYNUYOR", "TOPUKLA BIRAKIYOR"}
	golMesajlari := []string{"MÜTHİŞ BİR GOL", "KALECİ TOPU İZLEMEKTEN BAŞKA HİÇBİR ŞEY YAPAMADI", "AĞLARI DELİYOR", "TAM DOKSANA", "ÖRÜMCEK AĞLARINI AVLADI", "FİLELERİ HAVALANDIRIYOR", "ONA SADECE DOKUNMAK KALIYOR"}*/
	// var hucumSansFaktoru int
	// var defansSansFaktoru int
	path, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Kullanıcının home dizini bulunamadı.")
	}
	ilkTakim, ikinciTakim := OrnekIkiTakimOlustur()
	ilkTakim = TakimGucuHesapla(ilkTakim)
	ikinciTakim = TakimGucuHesapla(ikinciTakim)
	ilkTakim.GolSayisi = 0
	ikinciTakim.GolSayisi = 0

	/*fark := math.Abs(float64(ilkTakim.OrtalamaGuc) - float64(ikinciTakim.OrtalamaGuc))

	if fark > 5 {
		hucumSansFaktoru = 20
		defansSansFaktoru = 20
	} else {
		hucumSansFaktoru = 20
		defansSansFaktoru = 30
	}*/

	mac := YeniMacOlustur(&ilkTakim, &ikinciTakim)
	var takimAdi string
	var topaSahipOyuncu Futbolcu
	var seciliTaktik Taktik
	if mac.TopKimde == evSahibiSabiti {
		takimAdi = "ilkTakim"
		topaSahipOyuncu = mac.BaslangicOyuncusu()
		seciliTaktik = mac.EvSahibiTakim.TakimTaktik
	} else {
		takimAdi = "ikinciTakim"
		topaSahipOyuncu = mac.BaslangicOyuncusu()
		seciliTaktik = mac.DeplasmanTakimi.TakimTaktik
	}
	fmt.Println(mac.Dakika, mac.AnlikBolge, mac.TopKimde, mac.TopaSahipOyuncuIndex, topaSahipOyuncu.Isim)
	for i := 0; i <= 90; i++ {

		aksiyon := KararVer(topaSahipOyuncu, mac.AnlikBolge, seciliTaktik)

		if aksiyon == "Kısa Pas" {
			hedefOyuncuIndex := rand.Intn(11)
			for {
				if hedefOyuncuIndex == mac.TopaSahipOyuncuIndex || hedefOyuncuIndex > 10 || hedefOyuncuIndex < 0 {
					hedefOyuncuIndex = rand.Intn(11)
				} else {
					break
				}
			}
			rakipOyuncuIndex := rand.Intn(10) + 1
			var rakipOyuncu Futbolcu
			if takimAdi == "ilkTakim" {
				rakipOyuncu = ikinciTakim.Kadro[rakipOyuncuIndex]
			} else {
				rakipOyuncu = ilkTakim.Kadro[rakipOyuncuIndex]
			}
			basariIhtimali := topaSahipOyuncu.Profil.Pas + topaSahipOyuncu.Profil.Vizyon + topaSahipOyuncu.Profil.Teknik
			basarisizlikIhtimali := rakipOyuncu.Profil.DefansifPozisyonAlma + rakipOyuncu.Profil.TopKapma + rakipOyuncu.Profil.OnSezi
			toplamIhtimal := basariIhtimali + basarisizlikIhtimali
			zar := rand.Intn(toplamIhtimal)
			if zar < basariIhtimali {
				if takimAdi == "ilkTakim" {
					hedefOyuncu := ilkTakim.Kadro[hedefOyuncuIndex]
					geriPasIhtimali := 35
					yanPasIhtimali := 45
					ileriPasIhtimali := 20
					pasIhtimalleri := geriPasIhtimali + yanPasIhtimali + ileriPasIhtimali
					pasZar := rand.Intn(pasIhtimalleri)
					if pasZar < yanPasIhtimali {
					} else if pasZar < (yanPasIhtimali + geriPasIhtimali) {
						mac.AnlikBolge -= 1
					} else {
						mac.AnlikBolge += 1
					}
					if mac.AnlikBolge > 3 {
						mac.AnlikBolge = 3
					} else if mac.AnlikBolge < 1 {
						mac.AnlikBolge = 1
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s boştaki %s'e kısa oynuyor", i, topaSahipOyuncu.Isim, hedefOyuncu.Isim),
					}
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
					topaSahipOyuncu = hedefOyuncu
					mac.TopaSahipOyuncuIndex = hedefOyuncuIndex
				} else {
					hedefOyuncu := ikinciTakim.Kadro[hedefOyuncuIndex]

					geriPasIhtimali := 35
					yanPasIhtimali := 45
					ileriPasIhtimali := 20
					pasIhtimalleri := geriPasIhtimali + yanPasIhtimali + ileriPasIhtimali
					pasZar := rand.Intn(pasIhtimalleri)
					if pasZar < yanPasIhtimali {
					} else if pasZar < (yanPasIhtimali + geriPasIhtimali) {
						mac.AnlikBolge -= 1
					} else {
						mac.AnlikBolge += 1
					}
					if mac.AnlikBolge > 3 {
						mac.AnlikBolge = 3
					} else if mac.AnlikBolge < 1 {
						mac.AnlikBolge = 1
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s boştaki %s'e kısa oynuyor", i, topaSahipOyuncu.Isim, hedefOyuncu.Isim),
					}
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
					topaSahipOyuncu = hedefOyuncu
					mac.TopaSahipOyuncuIndex = hedefOyuncuIndex
				}
			} else {
				if takimAdi == "ilkTakim" {
					takimAdi = "ikinciTakim"
					seciliTaktik = ikinciTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s pas hatası! %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					bolgeCevirici := 4 - mac.AnlikBolge
					mac.AnlikBolge = int(bolgeCevirici)
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipOyuncu
					mac.TopaSahipOyuncuIndex = rakipOyuncuIndex
				} else {
					takimAdi = "ilkTakim"
					seciliTaktik = ilkTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s pas hatası! %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					bolgeCevirici := 4 - mac.AnlikBolge
					mac.AnlikBolge = int(bolgeCevirici)
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipOyuncu
					mac.TopaSahipOyuncuIndex = rakipOyuncuIndex
				}
			}
		} else if aksiyon == "Uzun Pas" || aksiyon == "Degaj" {
			hedefOyuncuIndex := rand.Intn(10) + 1
			for {
				if hedefOyuncuIndex == mac.TopaSahipOyuncuIndex || hedefOyuncuIndex > 10 || hedefOyuncuIndex < 0 {
					hedefOyuncuIndex = rand.Intn(10) + 1
				} else {
					break
				}
			}
			rakipOyuncuIndex := rand.Intn(10) + 1
			var rakipOyuncu Futbolcu
			if takimAdi == "ilkTakim" {
				rakipOyuncu = ikinciTakim.Kadro[rakipOyuncuIndex]
			} else {
				rakipOyuncu = ilkTakim.Kadro[rakipOyuncuIndex]
			}
			basariIhtimali := topaSahipOyuncu.Profil.Pas + topaSahipOyuncu.Profil.Vizyon + topaSahipOyuncu.Profil.Teknik
			basarisizlikIhtimali := rakipOyuncu.Profil.DefansifPozisyonAlma + rakipOyuncu.Profil.TopKapma + rakipOyuncu.Profil.KafaVurusu
			toplamIhtimal := basariIhtimali + basarisizlikIhtimali
			zar := rand.Intn(toplamIhtimal)
			if zar < basariIhtimali {
				if takimAdi == "ilkTakim" {
					hedefOyuncu := ilkTakim.Kadro[hedefOyuncuIndex]
					mac.AnlikBolge += 1
					if mac.AnlikBolge > 3 {
						mac.AnlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s boştaki %s'e uzun bir top", i, topaSahipOyuncu.Isim, hedefOyuncu.Isim),
					}
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
					topaSahipOyuncu = hedefOyuncu
					mac.TopaSahipOyuncuIndex = hedefOyuncuIndex
				} else {
					hedefOyuncu := ikinciTakim.Kadro[hedefOyuncuIndex]
					mac.AnlikBolge += 1
					if mac.AnlikBolge > 3 {
						mac.AnlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s boştaki %s'e uzun bir top", i, topaSahipOyuncu.Isim, hedefOyuncu.Isim),
					}
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
					topaSahipOyuncu = hedefOyuncu
					mac.TopaSahipOyuncuIndex = hedefOyuncuIndex
				}
			} else {
				if takimAdi == "ilkTakim" {
					takimAdi = "ikinciTakim"
					seciliTaktik = ikinciTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s pas hatası! %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					bolgeCevirici := 4 - mac.AnlikBolge
					mac.AnlikBolge = int(bolgeCevirici)
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipOyuncu
					mac.TopaSahipOyuncuIndex = rakipOyuncuIndex
				} else {
					takimAdi = "ilkTakim"
					seciliTaktik = ilkTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s pas hatası! %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					bolgeCevirici := 4 - mac.AnlikBolge
					mac.AnlikBolge = int(bolgeCevirici)
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipOyuncu
					mac.TopaSahipOyuncuIndex = rakipOyuncuIndex
				}
			}
		} else if aksiyon == "Erken Orta" || aksiyon == "Orta" {
			hedefOyuncuIndex := rand.Intn(10) + 1
			for {
				if hedefOyuncuIndex == mac.TopaSahipOyuncuIndex || hedefOyuncuIndex > 10 || hedefOyuncuIndex < 0 {
					hedefOyuncuIndex = rand.Intn(10) + 1
				} else {
					break
				}
			}
			rakipOyuncuIndex := rand.Intn(10) + 1
			var rakipOyuncu Futbolcu
			if takimAdi == "ilkTakim" {
				rakipOyuncu = ikinciTakim.Kadro[rakipOyuncuIndex]
			} else {
				rakipOyuncu = ilkTakim.Kadro[rakipOyuncuIndex]
			}
			basariIhtimali := topaSahipOyuncu.Profil.Pas + topaSahipOyuncu.Profil.OrtaYapma + topaSahipOyuncu.Profil.Teknik
			basarisizlikIhtimali := rakipOyuncu.Profil.DefansifPozisyonAlma + rakipOyuncu.Profil.TopKapma + rakipOyuncu.Profil.KafaVurusu
			toplamIhtimal := basariIhtimali + basarisizlikIhtimali
			zar := rand.Intn(toplamIhtimal)
			if zar < basariIhtimali {
				if takimAdi == "ilkTakim" {
					hedefOyuncu := ilkTakim.Kadro[hedefOyuncuIndex]
					mac.AnlikBolge += 1
					if mac.AnlikBolge > 3 {
						mac.AnlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s'den içerideki %s'e güzel bir orta", i, topaSahipOyuncu.Isim, hedefOyuncu.Isim),
					}
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
					topaSahipOyuncu = hedefOyuncu
					mac.TopaSahipOyuncuIndex = hedefOyuncuIndex
				} else {
					hedefOyuncu := ikinciTakim.Kadro[hedefOyuncuIndex]
					mac.AnlikBolge += 1
					if mac.AnlikBolge > 3 {
						mac.AnlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s'den içerideki %s'e güzel bir orta", i, topaSahipOyuncu.Isim, hedefOyuncu.Isim),
					}
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
					topaSahipOyuncu = hedefOyuncu
					mac.TopaSahipOyuncuIndex = hedefOyuncuIndex
				}
			} else {
				if takimAdi == "ilkTakim" {
					takimAdi = "ikinciTakim"
					seciliTaktik = ikinciTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s'den rezalet bir orta! %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					bolgeCevirici := 4 - mac.AnlikBolge
					mac.AnlikBolge = int(bolgeCevirici)
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipOyuncu
					mac.TopaSahipOyuncuIndex = rakipOyuncuIndex
				} else {
					takimAdi = "ilkTakim"
					seciliTaktik = ilkTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s'den rezalet bir orta! %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					bolgeCevirici := 4 - mac.AnlikBolge
					mac.AnlikBolge = int(bolgeCevirici)
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipOyuncu
					mac.TopaSahipOyuncuIndex = rakipOyuncuIndex
				}
			}
		} else if aksiyon == "Dikine Pas" || aksiyon == "Kilit Pas" {
			hedefOyuncuIndex := rand.Intn(10) + 1
			for {
				if hedefOyuncuIndex == mac.TopaSahipOyuncuIndex || hedefOyuncuIndex > 10 || hedefOyuncuIndex < 0 {
					hedefOyuncuIndex = rand.Intn(10) + 1
				} else {
					break
				}
			}
			rakipOyuncuIndex := rand.Intn(10) + 1
			var rakipOyuncu Futbolcu
			if takimAdi == "ilkTakim" {
				rakipOyuncu = ikinciTakim.Kadro[rakipOyuncuIndex]
			} else {
				rakipOyuncu = ilkTakim.Kadro[rakipOyuncuIndex]
			}
			basariIhtimali := topaSahipOyuncu.Profil.Pas + topaSahipOyuncu.Profil.Vizyon + topaSahipOyuncu.Profil.Teknik
			basarisizlikIhtimali := rakipOyuncu.Profil.DefansifPozisyonAlma + rakipOyuncu.Profil.TopKapma + rakipOyuncu.Profil.Markaj + 15
			toplamIhtimal := basariIhtimali + basarisizlikIhtimali
			zar := rand.Intn(toplamIhtimal)
			if zar < basariIhtimali {
				if takimAdi == "ilkTakim" {
					hedefOyuncu := ilkTakim.Kadro[hedefOyuncuIndex]
					mac.AnlikBolge += 1
					if mac.AnlikBolge > 3 {
						mac.AnlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s'den ilerideki %s'e müthiş bir pas.", i, topaSahipOyuncu.Isim, hedefOyuncu.Isim),
					}
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
					topaSahipOyuncu = hedefOyuncu
					mac.TopaSahipOyuncuIndex = hedefOyuncuIndex
				} else {
					hedefOyuncu := ikinciTakim.Kadro[hedefOyuncuIndex]
					mac.AnlikBolge += 1
					if mac.AnlikBolge > 3 {
						mac.AnlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s'den ilerideki %s'e müthiş bir pas.", i, topaSahipOyuncu.Isim, hedefOyuncu.Isim),
					}
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
					topaSahipOyuncu = hedefOyuncu
					mac.TopaSahipOyuncuIndex = hedefOyuncuIndex
				}
			} else {
				if takimAdi == "ilkTakim" {
					var metin string
					faulOlmaIhtimali := topaSahipOyuncu.Profil.KararAlma + topaSahipOyuncu.Profil.OnSezi
					temizOlmaIhtimali := rakipOyuncu.Profil.TopKapma + rakipOyuncu.Profil.Konsantrasyon + rakipOyuncu.Profil.KararAlma + 10
					toplamBasarisizIhtimal := faulOlmaIhtimali + temizOlmaIhtimali
					faulZari := rand.Intn(toplamBasarisizIhtimal)
					if faulZari < temizOlmaIhtimali {
						metin = fmt.Sprintf("%d. DAKİKA: %s'den araya iyi bir deneme.. Fakat %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim)
					} else {
						metin = fmt.Sprintf("%d. DAKİKA: %s'den araya iyi bir deneme.. %s'den sert müdahale, hakem düdüğünü çalıyor.", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim)
						kartsizGecmeIhtimali := 81
						sariKartIhtimali := 17
						kirmiziKartIhtimali := 2
						toplamKartIhtimali := kartsizGecmeIhtimali + sariKartIhtimali + kirmiziKartIhtimali
						kartZari := rand.Intn(toplamKartIhtimali)
						if kartZari < kartsizGecmeIhtimali {

						} else if kartZari < (kartsizGecmeIhtimali + sariKartIhtimali) {
							yasananAksiyon := Olay{
								Aksiyon: "Hakem elini cebine götürüyor, Sarı kart.",
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							rakipOyuncu.MactakiSariKartSayisi++
							if rakipOyuncu.MactakiSariKartSayisi >= 2 {
								yasananAksiyon := Olay{
									Aksiyon: "2. Sarısını görüyor, Takımı artık bir kişi eksik!",
								}
								mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
								ilkTakim.Kadro[rakipOyuncuIndex].MactaKirmizisiVarMi = true
							}
						} else {
							yasananAksiyon := Olay{
								Aksiyon: "DİREKT KIRMIZI KART! TAKIMI ARTIK BİR KİŞİ EKSİK MÜCADELE EDECEK!",
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							ilkTakim.Kadro[rakipOyuncuIndex].MactaKirmizisiVarMi = true
						}
					}
					takimAdi = "ikinciTakim"
					seciliTaktik = ikinciTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: metin,
					}
					bolgeCevirici := 4 - mac.AnlikBolge
					mac.AnlikBolge = int(bolgeCevirici)
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
					mac.TopaSahipOyuncuIndex = rand.Intn(5) + 6
					topaSahipOyuncu = ikinciTakim.Kadro[mac.TopaSahipOyuncuIndex]
					duranTopTuru := "Faul"
					if mac.AnlikBolge == 3 {
						penaltiIhtimali := 5
						penaltiOlmamaIhtimali := 95
						toplamPenaltiIhtimali := penaltiIhtimali + penaltiOlmamaIhtimali
						penaltiZari := rand.Intn(toplamPenaltiIhtimali)
						if penaltiZari < penaltiOlmamaIhtimali {
							duranTopTuru = "Faul"
						} else {
							duranTopTuru = "Penaltı"
							yasananAksiyon := Olay{
								Aksiyon: "Hakem beyaz noktayı gösteriyor, PENALTI!",
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
						}
					}
					DuranTop(topaSahipOyuncu, duranTopTuru, takimAdi, mac.AnlikBolge)
				} else {
					var metin string
					faulOlmaIhtimali := topaSahipOyuncu.Profil.KararAlma + topaSahipOyuncu.Profil.OnSezi
					temizOlmaIhtimali := rakipOyuncu.Profil.TopKapma + rakipOyuncu.Profil.Konsantrasyon + rakipOyuncu.Profil.KararAlma + 10
					toplamBasarisizIhtimal := faulOlmaIhtimali + temizOlmaIhtimali
					faulZari := rand.Intn(toplamBasarisizIhtimal)
					if faulZari < temizOlmaIhtimali {
						metin = fmt.Sprintf("%d. DAKİKA: %s'den araya iyi bir deneme.. Fakat %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim)
					} else {
						metin = fmt.Sprintf("%d. DAKİKA: %s'den araya iyi bir deneme.. %s'den sert müdahale, hakem düdüğünü çalıyor.", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim)
						kartsizGecmeIhtimali := 81
						sariKartIhtimali := 17
						kirmiziKartIhtimali := 2
						toplamKartIhtimali := kartsizGecmeIhtimali + sariKartIhtimali + kirmiziKartIhtimali
						kartZari := rand.Intn(toplamKartIhtimali)
						if kartZari < kartsizGecmeIhtimali {

						} else if kartZari < (kartsizGecmeIhtimali + sariKartIhtimali) {
							yasananAksiyon := Olay{
								Aksiyon: "Hakem elini cebine götürüyor, Sarı kart.",
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							rakipOyuncu.MactakiSariKartSayisi++
							if rakipOyuncu.MactakiSariKartSayisi >= 2 {
								yasananAksiyon := Olay{
									Aksiyon: "2. Sarısını görüyor, Takımı artık bir kişi eksik!",
								}
								mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
								ikinciTakim.Kadro[rakipOyuncuIndex].MactaKirmizisiVarMi = true
							}
						} else {
							yasananAksiyon := Olay{
								Aksiyon: "DİREKT KIRMIZI KART! TAKIMI ARTIK BİR KİŞİ EKSİK MÜCADELE EDECEK!",
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							ikinciTakim.Kadro[rakipOyuncuIndex].MactaKirmizisiVarMi = true
						}
					}
					takimAdi = "ilkTakim"
					seciliTaktik = ilkTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: metin,
					}
					bolgeCevirici := 4 - mac.AnlikBolge
					mac.AnlikBolge = int(bolgeCevirici)
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
					mac.TopaSahipOyuncuIndex = rand.Intn(5) + 6
					topaSahipOyuncu = ilkTakim.Kadro[mac.TopaSahipOyuncuIndex]
					duranTopTuru := "Faul"
					if mac.AnlikBolge == 3 {
						penaltiIhtimali := 5
						penaltiOlmamaIhtimali := 95
						toplamPenaltiIhtimali := penaltiIhtimali + penaltiOlmamaIhtimali
						penaltiZari := rand.Intn(toplamPenaltiIhtimali)
						if penaltiZari < penaltiOlmamaIhtimali {
							duranTopTuru = "Faul"
						} else {
							duranTopTuru = "Penaltı"
							yasananAksiyon := Olay{
								Aksiyon: "Hakem beyaz noktayı gösteriyor, PENALTI!",
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
						}
					}
					DuranTop(topaSahipOyuncu, duranTopTuru, takimAdi, mac.AnlikBolge)
				}
			}
		} else if aksiyon == "Dribling" {
			rakipOyuncuIndex := rand.Intn(10) + 1
			var rakipOyuncu Futbolcu
			if takimAdi == "ilkTakim" {
				rakipOyuncu = ikinciTakim.Kadro[rakipOyuncuIndex]
			} else {
				rakipOyuncu = ilkTakim.Kadro[rakipOyuncuIndex]
			}
			basariIhtimali := topaSahipOyuncu.Profil.Dribling + topaSahipOyuncu.Profil.Ceviklik + topaSahipOyuncu.Profil.Teknik + topaSahipOyuncu.Profil.IlkKontrol
			basarisizlikIhtimali := rakipOyuncu.Profil.DefansifPozisyonAlma + rakipOyuncu.Profil.TopKapma + rakipOyuncu.Profil.Markaj + 10
			toplamIhtimal := basariIhtimali + basarisizlikIhtimali
			zar := rand.Intn(toplamIhtimal)
			if zar < basariIhtimali {
				if takimAdi == "ilkTakim" {
					mac.AnlikBolge += 1
					if mac.AnlikBolge > 3 {
						mac.AnlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s'den nefis bir çalım %s dondu kaldı", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
				} else {
					mac.AnlikBolge += 1
					if mac.AnlikBolge > 3 {
						mac.AnlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s'den nefis bir çalım %s dondu kaldı", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
				}
			} else {
				if takimAdi == "ilkTakim" {
					var metin string
					faulOlmaIhtimali := topaSahipOyuncu.Profil.KararAlma + topaSahipOyuncu.Profil.OnSezi
					temizOlmaIhtimali := rakipOyuncu.Profil.TopKapma + rakipOyuncu.Profil.Konsantrasyon + rakipOyuncu.Profil.KararAlma + 10
					toplamBasarisizIhtimal := faulOlmaIhtimali + temizOlmaIhtimali
					faulZari := rand.Intn(toplamBasarisizIhtimal)
					if faulZari < temizOlmaIhtimali {
						metin = fmt.Sprintf("%d. DAKİKA: %s'den çalım denemesi.. Fakat %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim)
					} else {
						metin = fmt.Sprintf("%d. DAKİKA: %s'den çalım denemesi.. %s'den sert müdahale, hakem düdüğünü çalıyor.", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim)
						kartsizGecmeIhtimali := 81
						sariKartIhtimali := 17
						kirmiziKartIhtimali := 2
						toplamKartIhtimali := kartsizGecmeIhtimali + sariKartIhtimali + kirmiziKartIhtimali
						kartZari := rand.Intn(toplamKartIhtimali)
						if kartZari < kartsizGecmeIhtimali {

						} else if kartZari < (kartsizGecmeIhtimali + sariKartIhtimali) {
							yasananAksiyon := Olay{
								Aksiyon: "Hakem elini cebine götürüyor, Sarı kart.",
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							rakipOyuncu.MactakiSariKartSayisi++
							if rakipOyuncu.MactakiSariKartSayisi >= 2 {
								yasananAksiyon := Olay{
									Aksiyon: "2. Sarısını görüyor, Takımı artık bir kişi eksik!",
								}
								mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
								ilkTakim.Kadro[rakipOyuncuIndex].MactaKirmizisiVarMi = true
							}
						} else {
							yasananAksiyon := Olay{
								Aksiyon: "DİREKT KIRMIZI KART! TAKIMI ARTIK BİR KİŞİ EKSİK MÜCADELE EDECEK!",
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							ilkTakim.Kadro[rakipOyuncuIndex].MactaKirmizisiVarMi = true
						}
					}
					takimAdi = "ikinciTakim"
					seciliTaktik = ikinciTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: metin,
					}
					bolgeCevirici := 4 - mac.AnlikBolge
					mac.AnlikBolge = int(bolgeCevirici)
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
					mac.TopaSahipOyuncuIndex = rand.Intn(5) + 6
					topaSahipOyuncu = ikinciTakim.Kadro[mac.TopaSahipOyuncuIndex]
					duranTopTuru := "Faul"
					if mac.AnlikBolge == 3 {
						penaltiIhtimali := 5
						penaltiOlmamaIhtimali := 95
						toplamPenaltiIhtimali := penaltiIhtimali + penaltiOlmamaIhtimali
						penaltiZari := rand.Intn(toplamPenaltiIhtimali)
						if penaltiZari < penaltiOlmamaIhtimali {
							duranTopTuru = "Faul"
						} else {
							duranTopTuru = "Penaltı"
							yasananAksiyon := Olay{
								Aksiyon: "Hakem beyaz noktayı gösteriyor, PENALTI!",
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
						}
					}
					DuranTop(topaSahipOyuncu, duranTopTuru, takimAdi, mac.AnlikBolge)
				} else {
					var metin string
					faulOlmaIhtimali := topaSahipOyuncu.Profil.KararAlma + topaSahipOyuncu.Profil.OnSezi
					temizOlmaIhtimali := rakipOyuncu.Profil.TopKapma + rakipOyuncu.Profil.Konsantrasyon + rakipOyuncu.Profil.KararAlma + 10
					toplamBasarisizIhtimal := faulOlmaIhtimali + temizOlmaIhtimali
					faulZari := rand.Intn(toplamBasarisizIhtimal)
					if faulZari < temizOlmaIhtimali {
						metin = fmt.Sprintf("%d. DAKİKA: %s'den çalım denemesi.. Fakat %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim)
					} else {
						metin = fmt.Sprintf("%d. DAKİKA: %s'den çalım denemesi.. %s'den sert müdahale, hakem düdüğünü çalıyor.", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim)
						kartsizGecmeIhtimali := 81
						sariKartIhtimali := 17
						kirmiziKartIhtimali := 2
						toplamKartIhtimali := kartsizGecmeIhtimali + sariKartIhtimali + kirmiziKartIhtimali
						kartZari := rand.Intn(toplamKartIhtimali)
						if kartZari < kartsizGecmeIhtimali {

						} else if kartZari < (kartsizGecmeIhtimali + sariKartIhtimali) {
							yasananAksiyon := Olay{
								Aksiyon: "Hakem elini cebine götürüyor, Sarı kart.",
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							rakipOyuncu.MactakiSariKartSayisi++
							if rakipOyuncu.MactakiSariKartSayisi >= 2 {
								yasananAksiyon := Olay{
									Aksiyon: "2. Sarısını görüyor, Takımı artık bir kişi eksik!",
								}
								mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
								ikinciTakim.Kadro[rakipOyuncuIndex].MactaKirmizisiVarMi = true
							}
						} else {
							yasananAksiyon := Olay{
								Aksiyon: "DİREKT KIRMIZI KART! TAKIMI ARTIK BİR KİŞİ EKSİK MÜCADELE EDECEK!",
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							ikinciTakim.Kadro[rakipOyuncuIndex].MactaKirmizisiVarMi = true
						}
					}
					takimAdi = "ilkTakim"
					seciliTaktik = ilkTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: metin,
					}
					bolgeCevirici := 4 - mac.AnlikBolge
					mac.AnlikBolge = int(bolgeCevirici)
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
					mac.TopaSahipOyuncuIndex = rand.Intn(5) + 6
					topaSahipOyuncu = ilkTakim.Kadro[mac.TopaSahipOyuncuIndex]
					duranTopTuru := "Faul"
					if mac.AnlikBolge == 3 {
						penaltiIhtimali := 5
						penaltiOlmamaIhtimali := 95
						toplamPenaltiIhtimali := penaltiIhtimali + penaltiOlmamaIhtimali
						penaltiZari := rand.Intn(toplamPenaltiIhtimali)
						if penaltiZari < penaltiOlmamaIhtimali {
							duranTopTuru = "Faul"
						} else {
							duranTopTuru = "Penaltı"
							yasananAksiyon := Olay{
								Aksiyon: "Hakem beyaz noktayı gösteriyor, PENALTI!",
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
						}
					}
					DuranTop(topaSahipOyuncu, duranTopTuru, takimAdi, mac.AnlikBolge)
				}
			}
		} else if aksiyon == "Uzaktan Şut" {
			rakipOyuncuIndex := rand.Intn(10) + 1
			var rakipOyuncu Futbolcu
			var rakipKaleci Futbolcu
			if takimAdi == "ilkTakim" {
				rakipOyuncu = ikinciTakim.Kadro[rakipOyuncuIndex]
				rakipKaleci = ikinciTakim.Kadro[0]
			} else {
				rakipOyuncu = ilkTakim.Kadro[rakipOyuncuIndex]
				rakipKaleci = ilkTakim.Kadro[0]
			}
			basariIhtimali := topaSahipOyuncu.Profil.UzaktanSut + topaSahipOyuncu.Profil.Bitiricilik + topaSahipOyuncu.Profil.Teknik
			basarisizlikIhtimali := rakipOyuncu.Profil.DefansifPozisyonAlma + rakipOyuncu.Profil.TopKapma + rakipOyuncu.Profil.Markaj + 5 /* Kaleci İhtimalleri */ + rakipKaleci.Profil.Refleks + rakipKaleci.Profil.Konsantrasyon + 150
			toplamIhtimal := basariIhtimali + basarisizlikIhtimali
			zar := rand.Intn(toplamIhtimal)
			if zar < basariIhtimali {
				if takimAdi == "ilkTakim" {
					takimAdi = "ikinciTakim"
					seciliTaktik = ikinciTakim.TakimTaktik
					mac.AnlikBolge += 1
					if mac.AnlikBolge > 3 {
						mac.AnlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s Çok uzaktan kaleye bir füze yolluyor.. TOP AĞLARDA GOOOOOOOOOOOL %s'nin yapabilecek hiçbir şeyi yok", i, topaSahipOyuncu.Isim, rakipKaleci.Isim),
					}
					ilkTakim.GolSayisi++
					topaSahipOyuncu = ikinciTakim.Kadro[6]
					mac.TopaSahipOyuncuIndex = 6
					mac.AnlikBolge = 2
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
				} else {
					takimAdi = "ilkTakim"
					seciliTaktik = ilkTakim.TakimTaktik
					mac.AnlikBolge += 1
					if mac.AnlikBolge > 3 {
						mac.AnlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s Çok uzaktan kaleye bir füze yolluyor.. TOP AĞLARDA GOOOOOOOOOOOL %s'nin yapabilecek hiçbir şeyi yok", i, topaSahipOyuncu.Isim, rakipKaleci.Isim),
					}
					ikinciTakim.GolSayisi++
					topaSahipOyuncu = ilkTakim.Kadro[6]
					mac.TopaSahipOyuncuIndex = 6
					mac.AnlikBolge = 2
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
				}
			} else {
				if takimAdi == "ilkTakim" {
					bolgeCevirici := 4 - mac.AnlikBolge
					mac.AnlikBolge = int(bolgeCevirici)
					var metin string
					autIhtimali := 50
					kornerIhtimali := 25
					kalecideKalmaIhtimali := 12
					kalecidenDonmeIhtimali := 8
					direktenDonmeIhtimali := 5
					toplamBasarisizIhtimal := autIhtimali + kornerIhtimali + kalecideKalmaIhtimali + kalecidenDonmeIhtimali + direktenDonmeIhtimali
					basarisizlikZari := rand.Intn(toplamBasarisizIhtimal)
					if basarisizlikZari < autIhtimali {
						metin = fmt.Sprintf("%d. DAKİKA: %s Kaleyi uzaklardan yokluyor top az farkla auta gidiyor", i, topaSahipOyuncu.Isim)
						topaSahipOyuncu = rakipKaleci
						mac.TopaSahipOyuncuIndex = 0
						takimAdi = "ikinciTakim"
						seciliTaktik = ikinciTakim.TakimTaktik
					} else if basarisizlikZari < (autIhtimali + kornerIhtimali) {
						kaleciKurtarisIhtimali := 45
						defanstanDonmeIhtimali := 55
						kimdenDonduToplami := kaleciKurtarisIhtimali + defanstanDonmeIhtimali
						kimdenDonduZari := rand.Intn(kimdenDonduToplami)
						if kimdenDonduZari < defanstanDonmeIhtimali {
							donenOyuncuIndex := rand.Intn(8)
							var donenOyuncu Futbolcu
							donenOyuncu = ikinciTakim.Kadro[donenOyuncuIndex]
							metin = fmt.Sprintf("%d. DAKİKA: %s Kaleyi çok uzaktan yokluyor %s'den seken top kornere gidiyor", i, topaSahipOyuncu.Isim, donenOyuncu.Isim)
						} else {
							metin = fmt.Sprintf("%d. DAKİKA: %s Uzaklardan nefis bir şut %s aynı güzellikle bir kurtarışa imza atıyor. Korner", i, topaSahipOyuncu.Isim, rakipKaleci.Isim)
						}
						korneriKullanacakIndex := rand.Intn(4) + 6
						topaSahipOyuncu = ilkTakim.Kadro[korneriKullanacakIndex]
						mac.TopaSahipOyuncuIndex = korneriKullanacakIndex
						DuranTop(topaSahipOyuncu, "Korner", "ilkTakim", mac.AnlikBolge)

					} else if basarisizlikZari < (autIhtimali + kornerIhtimali + kalecideKalmaIhtimali) {
						metin = fmt.Sprintf("%d. DAKİKA: %s Çok uzaktan bir şut.. Fakat %s topu rahatça alıyor", i, topaSahipOyuncu.Isim, rakipKaleci.Isim)
						topaSahipOyuncu = rakipKaleci
						mac.TopaSahipOyuncuIndex = 0
						takimAdi = "ikinciTakim"
						seciliTaktik = ikinciTakim.TakimTaktik
					} else if basarisizlikZari < (autIhtimali + kornerIhtimali + kalecideKalmaIhtimali + kalecidenDonmeIhtimali) {
						metin = fmt.Sprintf("%d. DAKİKA: %s Çok uzaktan bir şut %s'den seken top boşta kaldı!", i, topaSahipOyuncu.Isim, rakipKaleci.Isim)
						hucumcuIndex := rand.Intn(5) + 6
						defansIndex := rand.Intn(7)
						var hucumOyuncusu Futbolcu
						var defansOyuncusu Futbolcu
						hucumOyuncusu = ilkTakim.Kadro[hucumcuIndex]
						defansOyuncusu = ikinciTakim.Kadro[defansIndex]
						hucumRibaundIhtimali := hucumOyuncusu.Profil.OnSezi + hucumOyuncusu.Profil.TopsuzAlan
						defansRibaundIhtimali := defansOyuncusu.Profil.DefansifPozisyonAlma + defansOyuncusu.Profil.OnSezi + 10
						toplamRibaundIhtimali := hucumRibaundIhtimali + defansRibaundIhtimali
						ribaundZari := rand.Intn(toplamRibaundIhtimali)
						if ribaundZari < hucumRibaundIhtimali {
							yasananAksiyon := Olay{
								Aksiyon: fmt.Sprintf("Boşta kalan topu %s alıyor! Atak devam edecek", hucumOyuncusu.Isim),
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							topaSahipOyuncu = hucumOyuncusu
							mac.TopaSahipOyuncuIndex = hucumcuIndex
						} else {
							yasananAksiyon := Olay{
								Aksiyon: fmt.Sprintf("Boşta kalan topu %s alıyor! Önemli bir atağı sonlandırdı", defansOyuncusu.Isim),
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							topaSahipOyuncu = defansOyuncusu
							mac.TopaSahipOyuncuIndex = defansIndex
							takimAdi = "ikinciTakim"
							seciliTaktik = ikinciTakim.TakimTaktik
						}
					} else {
						metin = fmt.Sprintf("%d. DAKİKA: %s Uzaklardan bir şut.. TOP DİREKTE PATLIYOR", i, topaSahipOyuncu.Isim)
						hucumcuIndex := rand.Intn(5) + 6
						defansIndex := rand.Intn(7)
						var hucumOyuncusu Futbolcu
						var defansOyuncusu Futbolcu
						hucumOyuncusu = ilkTakim.Kadro[hucumcuIndex]
						defansOyuncusu = ikinciTakim.Kadro[defansIndex]
						hucumRibaundIhtimali := hucumOyuncusu.Profil.OnSezi + hucumOyuncusu.Profil.TopsuzAlan
						defansRibaundIhtimali := defansOyuncusu.Profil.DefansifPozisyonAlma + defansOyuncusu.Profil.OnSezi + 10
						toplamRibaundIhtimali := hucumRibaundIhtimali + defansRibaundIhtimali
						ribaundZari := rand.Intn(toplamRibaundIhtimali)
						if ribaundZari < hucumRibaundIhtimali {
							yasananAksiyon := Olay{
								Aksiyon: fmt.Sprintf("Boşta kalan topu %s alıyor! Atak devam edecek", hucumOyuncusu.Isim),
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							topaSahipOyuncu = hucumOyuncusu
							mac.TopaSahipOyuncuIndex = hucumcuIndex
						} else {
							yasananAksiyon := Olay{
								Aksiyon: fmt.Sprintf("Boşta kalan topu %s alıyor! Önemli bir atağı sonlandırdı", defansOyuncusu.Isim),
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							topaSahipOyuncu = defansOyuncusu
							mac.TopaSahipOyuncuIndex = defansIndex
							takimAdi = "ikinciTakim"
							seciliTaktik = ikinciTakim.TakimTaktik
						}
					}
					yasananAksiyon := Olay{
						Aksiyon: metin,
					}
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
				} else {
					bolgeCevirici := 4 - mac.AnlikBolge
					mac.AnlikBolge = int(bolgeCevirici)
					var metin string
					autIhtimali := 50
					kornerIhtimali := 25
					kalecideKalmaIhtimali := 12
					kalecidenDonmeIhtimali := 8
					direktenDonmeIhtimali := 5
					toplamBasarisizIhtimal := autIhtimali + kornerIhtimali + kalecideKalmaIhtimali + kalecidenDonmeIhtimali + direktenDonmeIhtimali
					basarisizlikZari := rand.Intn(toplamBasarisizIhtimal)
					if basarisizlikZari < autIhtimali {
						metin = fmt.Sprintf("%d. DAKİKA: %s Çok uzaktan bir şut top az farkla auta gidiyor", i, topaSahipOyuncu.Isim)
						topaSahipOyuncu = rakipKaleci
						mac.TopaSahipOyuncuIndex = 0
						takimAdi = "ilkTakim"
						seciliTaktik = ilkTakim.TakimTaktik
					} else if basarisizlikZari < (autIhtimali + kornerIhtimali) {
						kaleciKurtarisIhtimali := 45
						defanstanDonmeIhtimali := 55
						kimdenDonduToplami := kaleciKurtarisIhtimali + defanstanDonmeIhtimali
						kimdenDonduZari := rand.Intn(kimdenDonduToplami)
						if kimdenDonduZari < defanstanDonmeIhtimali {
							donenOyuncuIndex := rand.Intn(8)
							var donenOyuncu Futbolcu
							donenOyuncu = ilkTakim.Kadro[donenOyuncuIndex]
							metin = fmt.Sprintf("%d. DAKİKA: %s Uzaklardan kaleyi yokluyor %s'den seken top kornere gidiyor", i, topaSahipOyuncu.Isim, donenOyuncu.Isim)
						} else {
							metin = fmt.Sprintf("%d. DAKİKA: %s Uzaklardan müthiş vuruyor %s aynı güzellikle bir kurtarışa imza atıyor. Korner", i, topaSahipOyuncu.Isim, rakipKaleci.Isim)
						}
						korneriKullanacakIndex := rand.Intn(4) + 6
						topaSahipOyuncu = ikinciTakim.Kadro[korneriKullanacakIndex]
						mac.TopaSahipOyuncuIndex = korneriKullanacakIndex
						DuranTop(topaSahipOyuncu, "Korner", "ikinciTakim", mac.AnlikBolge)
					} else if basarisizlikZari < (autIhtimali + kornerIhtimali + kalecideKalmaIhtimali) {
						metin = fmt.Sprintf("%d. DAKİKA: %s Çok uzaktan bir şut.. Fakat %s topu rahatça alıyor", i, topaSahipOyuncu.Isim, rakipKaleci.Isim)
						topaSahipOyuncu = rakipKaleci
						mac.TopaSahipOyuncuIndex = 0
						takimAdi = "ilkTakim"
						seciliTaktik = ilkTakim.TakimTaktik
					} else if basarisizlikZari < (autIhtimali + kornerIhtimali + kalecideKalmaIhtimali + kalecidenDonmeIhtimali) {
						metin = fmt.Sprintf("%d. DAKİKA: %s Çok uzaktan bir şut %s'den seken top boşta kaldı!", i, topaSahipOyuncu.Isim, rakipKaleci.Isim)
						hucumcuIndex := rand.Intn(5) + 6
						defansIndex := rand.Intn(7)
						var hucumOyuncusu Futbolcu
						var defansOyuncusu Futbolcu
						hucumOyuncusu = ikinciTakim.Kadro[hucumcuIndex]
						defansOyuncusu = ilkTakim.Kadro[defansIndex]
						hucumRibaundIhtimali := hucumOyuncusu.Profil.OnSezi + hucumOyuncusu.Profil.TopsuzAlan
						defansRibaundIhtimali := defansOyuncusu.Profil.DefansifPozisyonAlma + defansOyuncusu.Profil.OnSezi + 10
						toplamRibaundIhtimali := hucumRibaundIhtimali + defansRibaundIhtimali
						ribaundZari := rand.Intn(toplamRibaundIhtimali)
						if ribaundZari < hucumRibaundIhtimali {
							yasananAksiyon := Olay{
								Aksiyon: fmt.Sprintf("Boşta kalan topu %s alıyor! Atak devam edecek", hucumOyuncusu.Isim),
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							topaSahipOyuncu = hucumOyuncusu
							mac.TopaSahipOyuncuIndex = hucumcuIndex
						} else {
							yasananAksiyon := Olay{
								Aksiyon: fmt.Sprintf("Boşta kalan topu %s alıyor! Önemli bir atağı sonlandırdı", defansOyuncusu.Isim),
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							topaSahipOyuncu = defansOyuncusu
							mac.TopaSahipOyuncuIndex = defansIndex
							takimAdi = "ilkTakim"
							seciliTaktik = ilkTakim.TakimTaktik
						}
					} else {
						metin = fmt.Sprintf("%d. DAKİKA: %s Çok uzaktan bir şut.. TOP DİREKTE PATLIYOR", i, topaSahipOyuncu.Isim)
						hucumcuIndex := rand.Intn(5) + 6
						defansIndex := rand.Intn(7)
						var hucumOyuncusu Futbolcu
						var defansOyuncusu Futbolcu
						hucumOyuncusu = ikinciTakim.Kadro[hucumcuIndex]
						defansOyuncusu = ilkTakim.Kadro[defansIndex]
						hucumRibaundIhtimali := hucumOyuncusu.Profil.OnSezi + hucumOyuncusu.Profil.TopsuzAlan
						defansRibaundIhtimali := defansOyuncusu.Profil.DefansifPozisyonAlma + defansOyuncusu.Profil.OnSezi + 10
						toplamRibaundIhtimali := hucumRibaundIhtimali + defansRibaundIhtimali
						ribaundZari := rand.Intn(toplamRibaundIhtimali)
						if ribaundZari < hucumRibaundIhtimali {
							yasananAksiyon := Olay{
								Aksiyon: fmt.Sprintf("Boşta kalan topu %s alıyor! Atak devam edecek", hucumOyuncusu.Isim),
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							topaSahipOyuncu = hucumOyuncusu
							mac.TopaSahipOyuncuIndex = hucumcuIndex
						} else {
							yasananAksiyon := Olay{
								Aksiyon: fmt.Sprintf("Boşta kalan topu %s alıyor! Önemli bir atağı sonlandırdı", defansOyuncusu.Isim),
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							topaSahipOyuncu = defansOyuncusu
							mac.TopaSahipOyuncuIndex = defansIndex
							takimAdi = "ilkTakim"
							seciliTaktik = ilkTakim.TakimTaktik
						}
					}
					yasananAksiyon := Olay{
						Aksiyon: metin,
					}
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
				}
			}
		} else if aksiyon == "Şut" {
			rakipOyuncuIndex := rand.Intn(10) + 1
			var rakipOyuncu Futbolcu
			var rakipKaleci Futbolcu
			if takimAdi == "ilkTakim" {
				rakipOyuncu = ikinciTakim.Kadro[rakipOyuncuIndex]
				rakipKaleci = ikinciTakim.Kadro[0]
			} else {
				rakipOyuncu = ilkTakim.Kadro[rakipOyuncuIndex]
				rakipKaleci = ilkTakim.Kadro[0]
			}
			basariIhtimali := topaSahipOyuncu.Profil.OnSezi + topaSahipOyuncu.Profil.Bitiricilik + topaSahipOyuncu.Profil.Teknik + topaSahipOyuncu.Profil.KafaVurusu
			basarisizlikIhtimali := rakipOyuncu.Profil.DefansifPozisyonAlma + rakipOyuncu.Profil.TopKapma + rakipOyuncu.Profil.Markaj + 5 /* Kaleci İhtimalleri */ + rakipKaleci.Profil.Refleks + rakipKaleci.Profil.Konsantrasyon + 100
			toplamIhtimal := basariIhtimali + basarisizlikIhtimali
			zar := rand.Intn(toplamIhtimal)
			if zar < basariIhtimali {
				if takimAdi == "ilkTakim" {
					takimAdi = "ikinciTakim"
					seciliTaktik = ikinciTakim.TakimTaktik
					mac.AnlikBolge += 1
					if mac.AnlikBolge > 3 {
						mac.AnlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s Güzel bir şut.. TOP AĞLARDA GOOOOOOOOOOOL %s'nin yapabilecek hiçbir şeyi yok", i, topaSahipOyuncu.Isim, rakipKaleci.Isim),
					}
					ilkTakim.GolSayisi++
					topaSahipOyuncu = ikinciTakim.Kadro[6]
					mac.TopaSahipOyuncuIndex = 6
					mac.AnlikBolge = 2
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
				} else {
					takimAdi = "ilkTakim"
					seciliTaktik = ilkTakim.TakimTaktik
					mac.AnlikBolge += 1
					if mac.AnlikBolge > 3 {
						mac.AnlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s Güzel bir şut.. TOP AĞLARDA GOOOOOOOOOOOL %s'nin yapabilecek hiçbir şeyi yok", i, topaSahipOyuncu.Isim, rakipKaleci.Isim),
					}
					ikinciTakim.GolSayisi++
					topaSahipOyuncu = ilkTakim.Kadro[6]
					mac.TopaSahipOyuncuIndex = 6
					mac.AnlikBolge = 2
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
				}
			} else {
				if takimAdi == "ilkTakim" {
					bolgeCevirici := 4 - mac.AnlikBolge
					mac.AnlikBolge = int(bolgeCevirici)
					var metin string
					autIhtimali := 50
					kornerIhtimali := 25
					kalecideKalmaIhtimali := 12
					kalecidenDonmeIhtimali := 8
					direktenDonmeIhtimali := 5
					toplamBasarisizIhtimal := autIhtimali + kornerIhtimali + kalecideKalmaIhtimali + kalecidenDonmeIhtimali + direktenDonmeIhtimali
					basarisizlikZari := rand.Intn(toplamBasarisizIhtimal)
					if basarisizlikZari < autIhtimali {
						metin = fmt.Sprintf("%d. DAKİKA: %s'den bir şut top az farkla auta gidiyor", i, topaSahipOyuncu.Isim)
						topaSahipOyuncu = rakipKaleci
						mac.TopaSahipOyuncuIndex = 0
						takimAdi = "ikinciTakim"
						seciliTaktik = ikinciTakim.TakimTaktik
					} else if basarisizlikZari < (autIhtimali + kornerIhtimali) {
						kaleciKurtarisIhtimali := 45
						defanstanDonmeIhtimali := 55
						kimdenDonduToplami := kaleciKurtarisIhtimali + defanstanDonmeIhtimali
						kimdenDonduZari := rand.Intn(kimdenDonduToplami)
						if kimdenDonduZari < defanstanDonmeIhtimali {
							donenOyuncuIndex := rand.Intn(8)
							var donenOyuncu Futbolcu
							donenOyuncu = ikinciTakim.Kadro[donenOyuncuIndex]
							metin = fmt.Sprintf("%d. DAKİKA: %s'den nefis bir şut %s'den seken top kornere gidiyor", i, topaSahipOyuncu.Isim, donenOyuncu.Isim)
						} else {
							metin = fmt.Sprintf("%d. DAKİKA: %s'den nefis bir şut %s aynı güzellikle bir kurtarışa imza atıyor. Korner", i, topaSahipOyuncu.Isim, rakipKaleci.Isim)
						}
						korneriKullanacakIndex := rand.Intn(4) + 6
						topaSahipOyuncu = ilkTakim.Kadro[korneriKullanacakIndex]
						mac.TopaSahipOyuncuIndex = korneriKullanacakIndex
						DuranTop(topaSahipOyuncu, "Korner", "ilkTakim", mac.AnlikBolge)

					} else if basarisizlikZari < (autIhtimali + kornerIhtimali + kalecideKalmaIhtimali) {
						metin = fmt.Sprintf("%d. DAKİKA: %s Bir şut.. Fakat %s topu rahatça alıyor", i, topaSahipOyuncu.Isim, rakipKaleci.Isim)
						topaSahipOyuncu = rakipKaleci
						mac.TopaSahipOyuncuIndex = 0
						takimAdi = "ikinciTakim"
						seciliTaktik = ikinciTakim.TakimTaktik
					} else if basarisizlikZari < (autIhtimali + kornerIhtimali + kalecideKalmaIhtimali + kalecidenDonmeIhtimali) {
						metin = fmt.Sprintf("%d. DAKİKA: %s'den isabetli bir şut %s'den seken top boşta kaldı!", i, topaSahipOyuncu.Isim, rakipKaleci.Isim)
						hucumcuIndex := rand.Intn(5) + 6
						defansIndex := rand.Intn(7)
						var hucumOyuncusu Futbolcu
						var defansOyuncusu Futbolcu
						hucumOyuncusu = ilkTakim.Kadro[hucumcuIndex]
						defansOyuncusu = ikinciTakim.Kadro[defansIndex]
						hucumRibaundIhtimali := hucumOyuncusu.Profil.OnSezi + hucumOyuncusu.Profil.TopsuzAlan
						defansRibaundIhtimali := defansOyuncusu.Profil.DefansifPozisyonAlma + defansOyuncusu.Profil.OnSezi + 10
						toplamRibaundIhtimali := hucumRibaundIhtimali + defansRibaundIhtimali
						ribaundZari := rand.Intn(toplamRibaundIhtimali)
						if ribaundZari < hucumRibaundIhtimali {
							yasananAksiyon := Olay{
								Aksiyon: fmt.Sprintf("Boşta kalan topu %s alıyor! Atak devam edecek", hucumOyuncusu.Isim),
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							topaSahipOyuncu = hucumOyuncusu
							mac.TopaSahipOyuncuIndex = hucumcuIndex
						} else {
							yasananAksiyon := Olay{
								Aksiyon: fmt.Sprintf("Boşta kalan topu %s alıyor! Önemli bir atağı sonlandırdı", defansOyuncusu.Isim),
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							topaSahipOyuncu = defansOyuncusu
							mac.TopaSahipOyuncuIndex = defansIndex
							takimAdi = "ikinciTakim"
							seciliTaktik = ikinciTakim.TakimTaktik
						}
					} else {
						metin = fmt.Sprintf("%d. DAKİKA: %s Bir şut.. TOP DİREKTE PATLIYOR", i, topaSahipOyuncu.Isim)
						hucumcuIndex := rand.Intn(5) + 6
						defansIndex := rand.Intn(7)
						var hucumOyuncusu Futbolcu
						var defansOyuncusu Futbolcu
						hucumOyuncusu = ilkTakim.Kadro[hucumcuIndex]
						defansOyuncusu = ikinciTakim.Kadro[defansIndex]
						hucumRibaundIhtimali := hucumOyuncusu.Profil.OnSezi + hucumOyuncusu.Profil.TopsuzAlan
						defansRibaundIhtimali := defansOyuncusu.Profil.DefansifPozisyonAlma + defansOyuncusu.Profil.OnSezi + 10
						toplamRibaundIhtimali := hucumRibaundIhtimali + defansRibaundIhtimali
						ribaundZari := rand.Intn(toplamRibaundIhtimali)
						if ribaundZari < hucumRibaundIhtimali {
							yasananAksiyon := Olay{
								Aksiyon: fmt.Sprintf("Boşta kalan topu %s alıyor! Atak devam edecek", hucumOyuncusu.Isim),
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							topaSahipOyuncu = hucumOyuncusu
							mac.TopaSahipOyuncuIndex = hucumcuIndex
						} else {
							yasananAksiyon := Olay{
								Aksiyon: fmt.Sprintf("Boşta kalan topu %s alıyor! Önemli bir atağı sonlandırdı", defansOyuncusu.Isim),
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							topaSahipOyuncu = defansOyuncusu
							mac.TopaSahipOyuncuIndex = defansIndex
							takimAdi = "ikinciTakim"
							seciliTaktik = ikinciTakim.TakimTaktik
						}
					}
					yasananAksiyon := Olay{
						Aksiyon: metin,
					}
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
				} else {
					bolgeCevirici := 4 - mac.AnlikBolge
					mac.AnlikBolge = int(bolgeCevirici)
					var metin string
					autIhtimali := 50
					kornerIhtimali := 25
					kalecideKalmaIhtimali := 12
					kalecidenDonmeIhtimali := 8
					direktenDonmeIhtimali := 5
					toplamBasarisizIhtimal := autIhtimali + kornerIhtimali + kalecideKalmaIhtimali + kalecidenDonmeIhtimali + direktenDonmeIhtimali
					basarisizlikZari := rand.Intn(toplamBasarisizIhtimal)
					if basarisizlikZari < autIhtimali {
						metin = fmt.Sprintf("%d. DAKİKA: %s'den bir şut top az farkla auta gidiyor", i, topaSahipOyuncu.Isim)
						topaSahipOyuncu = rakipKaleci
						mac.TopaSahipOyuncuIndex = 0
						takimAdi = "ilkTakim"
						seciliTaktik = ilkTakim.TakimTaktik
					} else if basarisizlikZari < (autIhtimali + kornerIhtimali) {
						kaleciKurtarisIhtimali := 45
						defanstanDonmeIhtimali := 55
						kimdenDonduToplami := kaleciKurtarisIhtimali + defanstanDonmeIhtimali
						kimdenDonduZari := rand.Intn(kimdenDonduToplami)
						if kimdenDonduZari < defanstanDonmeIhtimali {
							donenOyuncuIndex := rand.Intn(8)
							var donenOyuncu Futbolcu
							donenOyuncu = ilkTakim.Kadro[donenOyuncuIndex]
							metin = fmt.Sprintf("%d. DAKİKA: %s'den nefis bir şut %s'den seken top kornere gidiyor", i, topaSahipOyuncu.Isim, donenOyuncu.Isim)
						} else {
							metin = fmt.Sprintf("%d. DAKİKA: %s'den nefis bir şut %s aynı güzellikle bir kurtarışa imza atıyor. Korner", i, topaSahipOyuncu.Isim, rakipKaleci.Isim)
						}
						korneriKullanacakIndex := rand.Intn(4) + 6
						topaSahipOyuncu = ikinciTakim.Kadro[korneriKullanacakIndex]
						mac.TopaSahipOyuncuIndex = korneriKullanacakIndex
						DuranTop(topaSahipOyuncu, "Korner", "ikinciTakim", mac.AnlikBolge)
					} else if basarisizlikZari < (autIhtimali + kornerIhtimali + kalecideKalmaIhtimali) {
						metin = fmt.Sprintf("%d. DAKİKA: %s Bir şut.. Fakat %s topu rahatça alıyor", i, topaSahipOyuncu.Isim, rakipKaleci.Isim)
						topaSahipOyuncu = rakipKaleci
						mac.TopaSahipOyuncuIndex = 0
						takimAdi = "ilkTakim"
						seciliTaktik = ilkTakim.TakimTaktik
					} else if basarisizlikZari < (autIhtimali + kornerIhtimali + kalecideKalmaIhtimali + kalecidenDonmeIhtimali) {
						metin = fmt.Sprintf("%d. DAKİKA: %s'den isabetli bir şut %s'den seken top boşta kaldı!", i, topaSahipOyuncu.Isim, rakipKaleci.Isim)
						hucumcuIndex := rand.Intn(5) + 6
						defansIndex := rand.Intn(7)
						var hucumOyuncusu Futbolcu
						var defansOyuncusu Futbolcu
						hucumOyuncusu = ikinciTakim.Kadro[hucumcuIndex]
						defansOyuncusu = ilkTakim.Kadro[defansIndex]
						hucumRibaundIhtimali := hucumOyuncusu.Profil.OnSezi + hucumOyuncusu.Profil.TopsuzAlan
						defansRibaundIhtimali := defansOyuncusu.Profil.DefansifPozisyonAlma + defansOyuncusu.Profil.OnSezi + 10
						toplamRibaundIhtimali := hucumRibaundIhtimali + defansRibaundIhtimali
						ribaundZari := rand.Intn(toplamRibaundIhtimali)
						if ribaundZari < hucumRibaundIhtimali {
							yasananAksiyon := Olay{
								Aksiyon: fmt.Sprintf("Boşta kalan topu %s alıyor! Atak devam edecek", hucumOyuncusu.Isim),
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							topaSahipOyuncu = hucumOyuncusu
							mac.TopaSahipOyuncuIndex = hucumcuIndex
						} else {
							yasananAksiyon := Olay{
								Aksiyon: fmt.Sprintf("Boşta kalan topu %s alıyor! Önemli bir atağı sonlandırdı", defansOyuncusu.Isim),
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							topaSahipOyuncu = defansOyuncusu
							mac.TopaSahipOyuncuIndex = defansIndex
							takimAdi = "ilkTakim"
							seciliTaktik = ilkTakim.TakimTaktik
						}
					} else {
						metin = fmt.Sprintf("%d. DAKİKA: %s Bir şut.. TOP DİREKTE PATLIYOR", i, topaSahipOyuncu.Isim)
						hucumcuIndex := rand.Intn(5) + 6
						defansIndex := rand.Intn(7)
						var hucumOyuncusu Futbolcu
						var defansOyuncusu Futbolcu
						hucumOyuncusu = ikinciTakim.Kadro[hucumcuIndex]
						defansOyuncusu = ilkTakim.Kadro[defansIndex]
						hucumRibaundIhtimali := hucumOyuncusu.Profil.OnSezi + hucumOyuncusu.Profil.TopsuzAlan
						defansRibaundIhtimali := defansOyuncusu.Profil.DefansifPozisyonAlma + defansOyuncusu.Profil.OnSezi + 10
						toplamRibaundIhtimali := hucumRibaundIhtimali + defansRibaundIhtimali
						ribaundZari := rand.Intn(toplamRibaundIhtimali)
						if ribaundZari < hucumRibaundIhtimali {
							yasananAksiyon := Olay{
								Aksiyon: fmt.Sprintf("Boşta kalan topu %s alıyor! Atak devam edecek", hucumOyuncusu.Isim),
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							topaSahipOyuncu = hucumOyuncusu
							mac.TopaSahipOyuncuIndex = hucumcuIndex
						} else {
							yasananAksiyon := Olay{
								Aksiyon: fmt.Sprintf("Boşta kalan topu %s alıyor! Önemli bir atağı sonlandırdı", defansOyuncusu.Isim),
							}
							mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
							topaSahipOyuncu = defansOyuncusu
							mac.TopaSahipOyuncuIndex = defansIndex
							takimAdi = "ilkTakim"
							seciliTaktik = ilkTakim.TakimTaktik
						}
					}
					yasananAksiyon := Olay{
						Aksiyon: metin,
					}
					mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
				}
			}
		}

		/*golMesajiIndex := rand.Intn(len(golMesajlari))
		asistMesajiIndex := rand.Intn(len(asistMesajlari))
		kacanMesajIndex := rand.Intn(len(kacanMesajlar))*/

		if i == 90 {
			skor := strconv.Itoa(ilkTakim.GolSayisi) + "-" + strconv.Itoa(ikinciTakim.GolSayisi)
			yasananAksiyon := Olay{
				Aksiyon: fmt.Sprintf("90 DAKİKA SONA ERDİ %s", skor),
			}
			mac.MacRaporu = append(mac.MacRaporu, yasananAksiyon)
		}
	}
	dondurulenListe, err := json.Marshal(mac.MacRaporu)
	if err != nil {
		fmt.Println("Liste marshal edilemedi")
		return
	}
	yol := filepath.Join(path, ".macSonucu.json")
	os.WriteFile(yol, dondurulenListe, 0644)
}

func KararVer(player Futbolcu, bolge int, takimTaktik Taktik) (aksiyon string) {

	if bolge == 1 {
		if player.Mevki == "Kaleci" {
			degajPuani := player.Profil.Degaj + player.Profil.Teknik + player.Profil.Vizyon
			kisaPasPuani := player.Profil.Pas + player.Profil.Sogukkanlilik + player.Profil.Cesaret
			if takimTaktik.KaleciOyunKurma == "Kısa Pas" {
				kisaPasPuani += 10
			} else if takimTaktik.KaleciOyunKurma == "Degaj" {
				degajPuani += 10
			}
			toplamIhtimal := degajPuani + kisaPasPuani
			yapilacakAksiyon := rand.Intn(toplamIhtimal)
			if yapilacakAksiyon < degajPuani {
				return "Degaj"
			} else {
				return "Kısa Pas"
			}
		} else if player.Mevki == "Stoper" {
			uzunPasPuani := player.Profil.Pas + player.Profil.Teknik + player.Profil.Vizyon
			kisaPasPuani := player.Profil.Pas + player.Profil.Sogukkanlilik + player.Profil.Cesaret + 5
			driblingPuani := player.Profil.Dribling + player.Profil.Denge + player.Profil.Teknik + player.Profil.IlkKontrol - player.Profil.TaktigeBaglilik - 5
			if takimTaktik.GeridenOyunKurma == "Kısa Pas" {
				kisaPasPuani += 10
			} else if takimTaktik.GeridenOyunKurma == "Uzun Pas" {
				uzunPasPuani += 10
			}
			if takimTaktik.DriblingIzni == "Teşvik Et" {
				driblingPuani += 5
			} else if takimTaktik.DriblingIzni == "Vazgeçir" {
				driblingPuani -= 10
			}
			toplamIhtimal := uzunPasPuani + kisaPasPuani + driblingPuani
			yapilacakAksiyon := rand.Intn(toplamIhtimal)
			if yapilacakAksiyon < uzunPasPuani {
				return "Uzun Pas"
			} else if yapilacakAksiyon < (uzunPasPuani + kisaPasPuani) {
				return "Kısa Pas"
			} else {
				return "Dribling"
			}
		} else if player.Mevki == "OrtaSaha" || player.Mevki == "DefansifOrtaSaha" || player.Mevki == "OfansifOrtaSaha" {
			uzunPasPuani := player.Profil.Pas + player.Profil.Teknik + player.Profil.Vizyon + 5
			kisaPasPuani := player.Profil.Pas + player.Profil.Sogukkanlilik + player.Profil.Cesaret
			driblingPuani := player.Profil.Dribling + player.Profil.Denge + player.Profil.Teknik + player.Profil.IlkKontrol - player.Profil.TaktigeBaglilik - 3
			if takimTaktik.GeridenOyunKurma == "Kısa Pas" {
				kisaPasPuani += 10
			} else if takimTaktik.GeridenOyunKurma == "Uzun Pas" {
				uzunPasPuani += 10
			}
			if takimTaktik.DriblingIzni == "Teşvik Et" {
				driblingPuani += 7
			} else if takimTaktik.DriblingIzni == "Vazgeçir" {
				driblingPuani -= 7
			}
			if driblingPuani <= 0 {
				driblingPuani = 1
			}
			toplamIhtimal := uzunPasPuani + kisaPasPuani + driblingPuani
			yapilacakAksiyon := rand.Intn(toplamIhtimal)
			if yapilacakAksiyon < uzunPasPuani {
				return "Uzun Pas"
			} else if yapilacakAksiyon < (uzunPasPuani + kisaPasPuani) {
				return "Kısa Pas"
			} else {
				return "Dribling"
			}
		} else if player.Mevki == "SagKanat" || player.Mevki == "SolKanat" || player.Mevki == "SagBek" || player.Mevki == "SolBek" {
			uzunPasPuani := player.Profil.Pas + player.Profil.Teknik + player.Profil.Vizyon + 3
			kisaPasPuani := player.Profil.Pas + player.Profil.Sogukkanlilik + player.Profil.Cesaret
			driblingPuani := player.Profil.Dribling + player.Profil.Denge + player.Profil.Teknik + player.Profil.IlkKontrol - player.Profil.TaktigeBaglilik + 5
			if takimTaktik.GeridenOyunKurma == "Kısa Pas" {
				kisaPasPuani += 10
			} else if takimTaktik.GeridenOyunKurma == "Uzun Pas" {
				uzunPasPuani += 10
			}
			if takimTaktik.DriblingIzni == "Teşvik Et" {
				driblingPuani += 10
			} else if takimTaktik.DriblingIzni == "Vazgeçir" {
				driblingPuani -= 10
			}
			if driblingPuani <= 0 {
				driblingPuani = 1
			}
			toplamIhtimal := uzunPasPuani + kisaPasPuani + driblingPuani
			yapilacakAksiyon := rand.Intn(toplamIhtimal)
			if yapilacakAksiyon < uzunPasPuani {
				return "Uzun Pas"
			} else if yapilacakAksiyon < (uzunPasPuani + kisaPasPuani) {
				return "Kısa Pas"
			} else {
				return "Dribling"
			}
		} else if player.Mevki == "Forvet" {
			uzunPasPuani := player.Profil.Pas + player.Profil.Teknik + player.Profil.Vizyon
			kisaPasPuani := player.Profil.Pas + player.Profil.Sogukkanlilik + player.Profil.Cesaret
			driblingPuani := player.Profil.Dribling + player.Profil.Denge + player.Profil.Teknik + player.Profil.IlkKontrol - player.Profil.TaktigeBaglilik
			if takimTaktik.GeridenOyunKurma == "Kısa Pas" {
				kisaPasPuani += 10
			} else if takimTaktik.GeridenOyunKurma == "Uzun Pas" {
				uzunPasPuani += 10
			}
			if takimTaktik.DriblingIzni == "Teşvik Et" {
				driblingPuani += 10
			} else if takimTaktik.DriblingIzni == "Vazgeçir" {
				driblingPuani -= 10
			}
			if driblingPuani <= 0 {
				driblingPuani = 1
			}
			toplamIhtimal := uzunPasPuani + kisaPasPuani + driblingPuani
			yapilacakAksiyon := rand.Intn(toplamIhtimal)
			if yapilacakAksiyon < uzunPasPuani {
				return "Uzun Pas"
			} else if yapilacakAksiyon < (uzunPasPuani + kisaPasPuani) {
				return "Kısa Pas"
			} else {
				return "Dribling"
			}
		}
	} else if bolge == 2 {
		if player.Mevki == "Kaleci" {
			degajPuani := player.Profil.Degaj + player.Profil.Teknik + player.Profil.Vizyon
			kisaPasPuani := player.Profil.Pas + player.Profil.Sogukkanlilik + player.Profil.Cesaret
			if takimTaktik.KaleciOyunKurma == "Kısa Pas" {
				kisaPasPuani += 10
			} else if takimTaktik.KaleciOyunKurma == "Degaj" {
				degajPuani += 10
			}
			toplamIhtimal := degajPuani + kisaPasPuani
			yapilacakAksiyon := rand.Intn(toplamIhtimal)
			if yapilacakAksiyon < degajPuani {
				return "Degaj"
			} else {
				return "Kısa Pas"
			}
		} else if player.Mevki == "Stoper" {
			dikinePasPuani := player.Profil.Pas + player.Profil.Teknik + player.Profil.Vizyon
			kisaPasPuani := player.Profil.Pas + player.Profil.Sogukkanlilik + player.Profil.Cesaret + 5
			driblingPuani := player.Profil.Dribling + player.Profil.Denge + player.Profil.Teknik + player.Profil.IlkKontrol - player.Profil.TaktigeBaglilik - 5
			if takimTaktik.GeridenOyunKurma == "Kısa Pas" {
				kisaPasPuani += 10
			} else if takimTaktik.GeridenOyunKurma == "Dikine Pas" {
				dikinePasPuani += 10
			}
			if takimTaktik.DriblingIzni == "Teşvik Et" {
				driblingPuani += 5
			} else if takimTaktik.DriblingIzni == "Vazgeçir" {
				driblingPuani -= 10
			}
			if driblingPuani <= 0 {
				driblingPuani = 1
			}
			toplamIhtimal := dikinePasPuani + kisaPasPuani + driblingPuani
			yapilacakAksiyon := rand.Intn(toplamIhtimal)
			if yapilacakAksiyon < dikinePasPuani {
				return "Dikine Pas"
			} else if yapilacakAksiyon < (dikinePasPuani + kisaPasPuani) {
				return "Kısa Pas"
			} else {
				return "Dribling"
			}
		} else if player.Mevki == "OrtaSaha" || player.Mevki == "DefansifOrtaSaha" || player.Mevki == "OfansifOrtaSaha" {
			dikinePasPuani := player.Profil.Pas + player.Profil.Teknik + player.Profil.Vizyon + 5
			kisaPasPuani := player.Profil.Pas + player.Profil.Sogukkanlilik + player.Profil.Cesaret
			driblingPuani := player.Profil.Dribling + player.Profil.Denge + player.Profil.Teknik + player.Profil.IlkKontrol - player.Profil.TaktigeBaglilik
			uzaktanSutPuani := player.Profil.UzaktanSut + player.Profil.Bitiricilik + player.Profil.KararAlma + player.Profil.Teknik - player.Profil.TaktigeBaglilik - 5
			if takimTaktik.OyunKurma == "Kısa Pas" {
				kisaPasPuani += 10
			} else if takimTaktik.OyunKurma == "Dikine Pas" {
				dikinePasPuani += 10
			}
			if takimTaktik.DriblingIzni == "Teşvik Et" {
				driblingPuani += 10
			} else if takimTaktik.DriblingIzni == "Vazgeçir" {
				driblingPuani -= 5
			}
			if takimTaktik.UzaktanSut == "Teşvik Et" {
				uzaktanSutPuani += 5
			} else if takimTaktik.UzaktanSut == "Vazgeçir" {
				uzaktanSutPuani -= 10
			}
			if driblingPuani <= 0 {
				driblingPuani = 1
			}
			if uzaktanSutPuani <= 0 {
				uzaktanSutPuani = 1
			}
			toplamIhtimal := dikinePasPuani + kisaPasPuani + driblingPuani + uzaktanSutPuani
			yapilacakAksiyon := rand.Intn(toplamIhtimal)
			if yapilacakAksiyon < dikinePasPuani {
				return "Dikine Pas"
			} else if yapilacakAksiyon < (dikinePasPuani + kisaPasPuani) {
				return "Kısa Pas"
			} else if yapilacakAksiyon < (dikinePasPuani + kisaPasPuani + driblingPuani) {
				return "Dribling"
			} else {
				return "Uzaktan Şut"
			}
		} else if player.Mevki == "SagKanat" || player.Mevki == "SolKanat" {
			dikinePasPuani := player.Profil.Pas + player.Profil.Teknik + player.Profil.Vizyon + 3
			kisaPasPuani := player.Profil.Pas + player.Profil.Sogukkanlilik + player.Profil.Cesaret
			driblingPuani := player.Profil.Dribling + player.Profil.Denge + player.Profil.Teknik + player.Profil.IlkKontrol - player.Profil.TaktigeBaglilik + 3
			uzaktanSutPuani := player.Profil.UzaktanSut + player.Profil.Bitiricilik + player.Profil.KararAlma + player.Profil.Teknik - player.Profil.TaktigeBaglilik - 3
			ortaPuani := player.Profil.OrtaYapma + player.Profil.Teknik + player.Profil.Pas - 10
			if takimTaktik.OyunKurma == "Kısa Pas" {
				kisaPasPuani += 10
			} else if takimTaktik.OyunKurma == "Dikine Pas" {
				dikinePasPuani += 10
			}
			if takimTaktik.DriblingIzni == "Teşvik Et" {
				driblingPuani += 10
			} else if takimTaktik.DriblingIzni == "Vazgeçir" {
				driblingPuani -= 10
			}
			if takimTaktik.UzaktanSut == "Teşvik Et" {
				uzaktanSutPuani += 5
			} else if takimTaktik.UzaktanSut == "Vazgeçir" {
				uzaktanSutPuani -= 5
			}
			if takimTaktik.Orta == "Erken Orta" {
				ortaPuani += 3
			} else {
				ortaPuani -= 5
			}
			if ortaPuani <= 0 {
				ortaPuani = 1
			}
			if driblingPuani <= 0 {
				driblingPuani = 1
			}
			if uzaktanSutPuani <= 0 {
				uzaktanSutPuani = 1
			}
			toplamIhtimal := dikinePasPuani + kisaPasPuani + driblingPuani + uzaktanSutPuani + ortaPuani
			yapilacakAksiyon := rand.Intn(toplamIhtimal)
			if yapilacakAksiyon < dikinePasPuani {
				return "Dikine Pas"
			} else if yapilacakAksiyon < (dikinePasPuani + kisaPasPuani) {
				return "Kısa Pas"
			} else if yapilacakAksiyon < (dikinePasPuani + kisaPasPuani + driblingPuani) {
				return "Dribling"
			} else if yapilacakAksiyon < (dikinePasPuani + kisaPasPuani + driblingPuani + uzaktanSutPuani) {
				return "Uzaktan Şut"
			} else {
				return "Erken Orta"
			}
		} else if player.Mevki == "SagBek" || player.Mevki == "SolBek" {
			dikinePasPuani := player.Profil.Pas + player.Profil.Teknik + player.Profil.Vizyon
			kisaPasPuani := player.Profil.Pas + player.Profil.Sogukkanlilik + player.Profil.Cesaret + 3
			driblingPuani := player.Profil.Dribling + player.Profil.Denge + player.Profil.Teknik + player.Profil.IlkKontrol - player.Profil.TaktigeBaglilik
			uzaktanSutPuani := player.Profil.UzaktanSut + player.Profil.Bitiricilik + player.Profil.KararAlma + player.Profil.Teknik - player.Profil.TaktigeBaglilik - 5
			ortaPuani := player.Profil.OrtaYapma + player.Profil.Teknik + player.Profil.Pas - 10
			if takimTaktik.OyunKurma == "Kısa Pas" {
				kisaPasPuani += 10
			} else if takimTaktik.OyunKurma == "Dikine Pas" {
				dikinePasPuani += 10
			}
			if takimTaktik.DriblingIzni == "Teşvik Et" {
				driblingPuani += 10
			} else if takimTaktik.DriblingIzni == "Vazgeçir" {
				driblingPuani -= 10
			}
			if takimTaktik.UzaktanSut == "Teşvik Et" {
				uzaktanSutPuani += 3
			} else if takimTaktik.UzaktanSut == "Vazgeçir" {
				uzaktanSutPuani -= 7
			}
			if takimTaktik.Orta == "Erken Orta" {
				ortaPuani += 3
			} else {
				ortaPuani -= 5
			}
			if ortaPuani <= 0 {
				ortaPuani = 1
			}
			if driblingPuani <= 0 {
				driblingPuani = 1
			}
			if uzaktanSutPuani <= 0 {
				uzaktanSutPuani = 1
			}
			toplamIhtimal := dikinePasPuani + kisaPasPuani + driblingPuani + uzaktanSutPuani + ortaPuani
			yapilacakAksiyon := rand.Intn(toplamIhtimal)
			if yapilacakAksiyon < dikinePasPuani {
				return "Dikine Pas"
			} else if yapilacakAksiyon < (dikinePasPuani + kisaPasPuani) {
				return "Kısa Pas"
			} else if yapilacakAksiyon < (dikinePasPuani + kisaPasPuani + driblingPuani) {
				return "Dribling"
			} else if yapilacakAksiyon < (dikinePasPuani + kisaPasPuani + driblingPuani + uzaktanSutPuani) {
				return "Uzaktan Şut"
			} else {
				return "Erken Orta"
			}
		} else if player.Mevki == "Forvet" {
			dikinePasPuani := player.Profil.Pas + player.Profil.Teknik + player.Profil.Vizyon
			kisaPasPuani := player.Profil.Pas + player.Profil.Sogukkanlilik + player.Profil.Cesaret + 3
			driblingPuani := player.Profil.Dribling + player.Profil.Denge + player.Profil.Teknik + player.Profil.IlkKontrol - player.Profil.TaktigeBaglilik
			uzaktanSutPuani := player.Profil.UzaktanSut + player.Profil.Bitiricilik + player.Profil.KararAlma + player.Profil.Teknik - player.Profil.TaktigeBaglilik - 3
			if takimTaktik.OyunKurma == "Kısa Pas" {
				kisaPasPuani += 10
			} else if takimTaktik.OyunKurma == "Dikine Pas" {
				dikinePasPuani += 10
			}
			if takimTaktik.DriblingIzni == "Teşvik Et" {
				driblingPuani += 10
			} else if takimTaktik.DriblingIzni == "Vazgeçir" {
				driblingPuani -= 10
			}
			if takimTaktik.UzaktanSut == "Teşvik Et" {
				uzaktanSutPuani += 7
			} else if takimTaktik.UzaktanSut == "Vazgeçir" {
				uzaktanSutPuani -= 7
			}
			if driblingPuani <= 0 {
				driblingPuani = 1
			}
			if uzaktanSutPuani <= 0 {
				uzaktanSutPuani = 1
			}
			toplamIhtimal := dikinePasPuani + kisaPasPuani + driblingPuani + uzaktanSutPuani
			yapilacakAksiyon := rand.Intn(toplamIhtimal)
			if yapilacakAksiyon < dikinePasPuani {
				return "Dikine Pas"
			} else if yapilacakAksiyon < (dikinePasPuani + kisaPasPuani) {
				return "Kısa Pas"
			} else if yapilacakAksiyon < (dikinePasPuani + kisaPasPuani + driblingPuani) {
				return "Dribling"
			} else {
				return "Uzaktan Şut"
			}
		}
	} else if bolge == 3 {
		if player.Mevki == "Stoper" {
			kilitPasPuani := player.Profil.Pas + player.Profil.Teknik + player.Profil.Vizyon
			kisaPasPuani := player.Profil.Pas + player.Profil.Sogukkanlilik + player.Profil.Cesaret + 5
			driblingPuani := player.Profil.Dribling + player.Profil.Denge + player.Profil.Teknik + player.Profil.IlkKontrol - player.Profil.TaktigeBaglilik - 5
			sutPuani := player.Profil.Bitiricilik + player.Profil.Cesaret + player.Profil.KararAlma + player.Profil.Teknik - player.Profil.TaktigeBaglilik - 5
			if takimTaktik.GeridenOyunKurma == "Kısa Pas" {
				kisaPasPuani += 10
			} else if takimTaktik.GeridenOyunKurma == "Dikine Pas" {
				kilitPasPuani += 10
			}
			if takimTaktik.DriblingIzni == "Teşvik Et" {
				driblingPuani += 5
			} else if takimTaktik.DriblingIzni == "Vazgeçir" {
				driblingPuani -= 10
			}
			if driblingPuani <= 0 {
				driblingPuani = 1
			}
			toplamIhtimal := kilitPasPuani + kisaPasPuani + driblingPuani + sutPuani
			yapilacakAksiyon := rand.Intn(toplamIhtimal)
			if yapilacakAksiyon < kilitPasPuani {
				return "Kilit Pas"
			} else if yapilacakAksiyon < (kilitPasPuani + kisaPasPuani) {
				return "Kısa Pas"
			} else if yapilacakAksiyon < (kilitPasPuani + kisaPasPuani + driblingPuani) {
				return "Dribling"
			} else {
				return "Şut"
			}
		} else if player.Mevki == "OrtaSaha" || player.Mevki == "DefansifOrtaSaha" || player.Mevki == "OfansifOrtaSaha" {
			kilitPasPuani := player.Profil.Pas + player.Profil.Teknik + player.Profil.Vizyon + 5
			kisaPasPuani := player.Profil.Pas + player.Profil.Sogukkanlilik + player.Profil.Cesaret - 3
			driblingPuani := player.Profil.Dribling + player.Profil.Denge + player.Profil.Teknik + player.Profil.IlkKontrol - player.Profil.TaktigeBaglilik
			uzaktanSutPuani := player.Profil.UzaktanSut + player.Profil.Bitiricilik + player.Profil.KararAlma + player.Profil.Teknik - player.Profil.TaktigeBaglilik
			sutPuani := player.Profil.Bitiricilik + player.Profil.KararAlma + player.Profil.Teknik - player.Profil.TaktigeBaglilik + 3 + player.Profil.Sogukkanlilik
			if takimTaktik.OyunKurma == "Kısa Pas" {
				kisaPasPuani += 10
			} else if takimTaktik.OyunKurma == "Dikine Pas" {
				kilitPasPuani += 10
			}
			if takimTaktik.DriblingIzni == "Teşvik Et" {
				driblingPuani += 10
			} else if takimTaktik.DriblingIzni == "Vazgeçir" {
				driblingPuani -= 5
			}
			if takimTaktik.UzaktanSut == "Teşvik Et" {
				uzaktanSutPuani += 7
			} else if takimTaktik.UzaktanSut == "Vazgeçir" {
				uzaktanSutPuani -= 7
			}
			if driblingPuani <= 0 {
				driblingPuani = 1
			}
			if uzaktanSutPuani <= 0 {
				uzaktanSutPuani = 1
			}
			toplamIhtimal := kilitPasPuani + kisaPasPuani + driblingPuani + uzaktanSutPuani + sutPuani
			yapilacakAksiyon := rand.Intn(toplamIhtimal)
			if yapilacakAksiyon < kilitPasPuani {
				return "Kilit Pas"
			} else if yapilacakAksiyon < (kilitPasPuani + kisaPasPuani) {
				return "Kısa Pas"
			} else if yapilacakAksiyon < (kilitPasPuani + kisaPasPuani + driblingPuani) {
				return "Dribling"
			} else if yapilacakAksiyon < (kilitPasPuani + kisaPasPuani + driblingPuani + uzaktanSutPuani) {
				return "Uzaktan Şut"
			} else {
				return "Şut"
			}
		} else if player.Mevki == "SagKanat" || player.Mevki == "SolKanat" {
			kilitPasPuani := player.Profil.Pas + player.Profil.Teknik + player.Profil.Vizyon + 5
			kisaPasPuani := player.Profil.Pas + player.Profil.Sogukkanlilik + player.Profil.Cesaret - 3
			driblingPuani := player.Profil.Dribling + player.Profil.Denge + player.Profil.Teknik + player.Profil.IlkKontrol - player.Profil.TaktigeBaglilik + 5
			uzaktanSutPuani := player.Profil.UzaktanSut + player.Profil.Bitiricilik + player.Profil.KararAlma + player.Profil.Teknik - player.Profil.TaktigeBaglilik + 3
			sutPuani := player.Profil.Bitiricilik + player.Profil.KararAlma + player.Profil.Teknik - player.Profil.TaktigeBaglilik + 3 + player.Profil.Sogukkanlilik
			ortaPuani := player.Profil.OrtaYapma + player.Profil.Teknik + player.Profil.Pas + 5
			if takimTaktik.OyunKurma == "Kısa Pas" {
				kisaPasPuani += 10
			} else if takimTaktik.OyunKurma == "Dikine Pas" {
				kilitPasPuani += 10
			}
			if takimTaktik.DriblingIzni == "Teşvik Et" {
				driblingPuani += 10
			} else if takimTaktik.DriblingIzni == "Vazgeçir" {
				driblingPuani -= 10
			}
			if takimTaktik.UzaktanSut == "Teşvik Et" {
				uzaktanSutPuani += 5
			} else if takimTaktik.UzaktanSut == "Vazgeçir" {
				uzaktanSutPuani -= 5
			}
			if takimTaktik.Orta == "Erken Orta" {
				ortaPuani += 3
			} else {
				ortaPuani -= 5
			}
			if ortaPuani <= 0 {
				ortaPuani = 1
			}
			if driblingPuani <= 0 {
				driblingPuani = 1
			}
			if uzaktanSutPuani <= 0 {
				uzaktanSutPuani = 1
			}
			toplamIhtimal := kilitPasPuani + kisaPasPuani + driblingPuani + uzaktanSutPuani + ortaPuani + sutPuani
			yapilacakAksiyon := rand.Intn(toplamIhtimal)
			if yapilacakAksiyon < kilitPasPuani {
				return "Kilit Pas"
			} else if yapilacakAksiyon < (kilitPasPuani + kisaPasPuani) {
				return "Kısa Pas"
			} else if yapilacakAksiyon < (kilitPasPuani + kisaPasPuani + driblingPuani) {
				return "Dribling"
			} else if yapilacakAksiyon < (kilitPasPuani + kisaPasPuani + driblingPuani + uzaktanSutPuani) {
				return "Uzaktan Şut"
			} else if yapilacakAksiyon < (kilitPasPuani + kisaPasPuani + driblingPuani + uzaktanSutPuani + ortaPuani) {
				return "Orta"
			} else {
				return "Şut"
			}
		} else if player.Mevki == "SagBek" || player.Mevki == "SolBek" {
			kilitPasPuani := player.Profil.Pas + player.Profil.Teknik + player.Profil.Vizyon + 3
			kisaPasPuani := player.Profil.Pas + player.Profil.Sogukkanlilik + player.Profil.Cesaret
			driblingPuani := player.Profil.Dribling + player.Profil.Denge + player.Profil.Teknik + player.Profil.IlkKontrol - player.Profil.TaktigeBaglilik + 3
			uzaktanSutPuani := player.Profil.UzaktanSut + player.Profil.Bitiricilik + player.Profil.KararAlma + player.Profil.Teknik - player.Profil.TaktigeBaglilik - 3
			ortaPuani := player.Profil.OrtaYapma + player.Profil.Teknik + player.Profil.Pas + 5
			sutPuani := player.Profil.Bitiricilik + player.Profil.KararAlma + player.Profil.Teknik - player.Profil.TaktigeBaglilik + player.Profil.Sogukkanlilik - 3
			if takimTaktik.OyunKurma == "Kısa Pas" {
				kisaPasPuani += 10
			} else if takimTaktik.OyunKurma == "Dikine Pas" {
				kilitPasPuani += 10
			}
			if takimTaktik.DriblingIzni == "Teşvik Et" {
				driblingPuani += 10
			} else if takimTaktik.DriblingIzni == "Vazgeçir" {
				driblingPuani -= 10
			}
			if takimTaktik.UzaktanSut == "Teşvik Et" {
				uzaktanSutPuani += 3
			} else if takimTaktik.UzaktanSut == "Vazgeçir" {
				uzaktanSutPuani -= 7
			}
			if takimTaktik.Orta == "Erken Orta" {
				ortaPuani += 3
			} else {
				ortaPuani -= 5
			}
			if ortaPuani <= 0 {
				ortaPuani = 1
			}
			if driblingPuani <= 0 {
				driblingPuani = 1
			}
			if uzaktanSutPuani <= 0 {
				uzaktanSutPuani = 1
			}
			toplamIhtimal := kilitPasPuani + kisaPasPuani + driblingPuani + uzaktanSutPuani + ortaPuani + sutPuani
			yapilacakAksiyon := rand.Intn(toplamIhtimal)
			if yapilacakAksiyon < kilitPasPuani {
				return "Kilit Pas"
			} else if yapilacakAksiyon < (kilitPasPuani + kisaPasPuani) {
				return "Kısa Pas"
			} else if yapilacakAksiyon < (kilitPasPuani + kisaPasPuani + driblingPuani) {
				return "Dribling"
			} else if yapilacakAksiyon < (kilitPasPuani + kisaPasPuani + driblingPuani + uzaktanSutPuani) {
				return "Uzaktan Şut"
			} else if yapilacakAksiyon < (kilitPasPuani + kisaPasPuani + driblingPuani + uzaktanSutPuani + ortaPuani) {
				return "Orta"
			} else {
				return "Şut"
			}
		} else if player.Mevki == "Forvet" {
			kilitPasPuani := player.Profil.Pas + player.Profil.Teknik + player.Profil.Vizyon
			kisaPasPuani := player.Profil.Pas + player.Profil.Sogukkanlilik + player.Profil.Cesaret + 3
			driblingPuani := player.Profil.Dribling + player.Profil.Denge + player.Profil.Teknik + player.Profil.IlkKontrol - player.Profil.TaktigeBaglilik
			uzaktanSutPuani := player.Profil.UzaktanSut + player.Profil.Bitiricilik + player.Profil.KararAlma + player.Profil.Teknik - player.Profil.TaktigeBaglilik
			sutPuani := player.Profil.Bitiricilik + player.Profil.KararAlma + player.Profil.Teknik - player.Profil.TaktigeBaglilik + player.Profil.Sogukkanlilik + 5
			if takimTaktik.OyunKurma == "Kısa Pas" {
				kisaPasPuani += 10
			} else if takimTaktik.OyunKurma == "Dikine Pas" {
				kilitPasPuani += 10
			}
			if takimTaktik.DriblingIzni == "Teşvik Et" {
				driblingPuani += 10
			} else if takimTaktik.DriblingIzni == "Vazgeçir" {
				driblingPuani -= 10
			}
			if takimTaktik.UzaktanSut == "Teşvik Et" {
				uzaktanSutPuani += 7
			} else if takimTaktik.UzaktanSut == "Vazgeçir" {
				uzaktanSutPuani -= 7
			}
			if driblingPuani <= 0 {
				driblingPuani = 1
			}
			if uzaktanSutPuani <= 0 {
				uzaktanSutPuani = 1
			}
			toplamIhtimal := kilitPasPuani + kisaPasPuani + driblingPuani + uzaktanSutPuani + sutPuani
			yapilacakAksiyon := rand.Intn(toplamIhtimal)
			if yapilacakAksiyon < kilitPasPuani {
				return "Kilit Pas"
			} else if yapilacakAksiyon < (kilitPasPuani + kisaPasPuani) {
				return "Kısa Pas"
			} else if yapilacakAksiyon < (kilitPasPuani + kisaPasPuani + driblingPuani) {
				return "Dribling"
			} else if yapilacakAksiyon < (kilitPasPuani + kisaPasPuani + driblingPuani + uzaktanSutPuani) {
				return "Uzaktan Şut"
			} else {
				return "Şut"
			}
		}
	}
	return "Bekle"
}

func DuranTop(kullananOyuncu Futbolcu, duranTopTuru string, hangiTakim string, bolge int) {
	// boş kal şimdilik
}

func sinirla(deger int) int {
	if deger > 20 {
		return 20
	}
	if deger < 1 {
		return 1
	}
	return deger
}
