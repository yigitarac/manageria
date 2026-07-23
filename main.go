package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
)

type Taktik struct {
	KaleciOyunKurma  string
	GeridenOyunKurma string
	OyunKurma        string
	DriblingIzni     string
	UzaktanSut       string
	Orta             string
}

type Ozellikler struct {
	Bitiricilik          int
	UzaktanSut           int
	OnSezi               int
	Dribling             int
	DuranToplar          int
	Pas                  int
	OrtaYapma            int
	Teknik               int
	IlkKontrol           int
	Vizyon               int
	Markaj               int
	TopKapma             int
	KafaVurusu           int
	DefansifPozisyonAlma int
	KararAlma            int
	TopsuzAlan           int
	Caliskanlik          int
	Kararlilik           int
	Cesaret              int
	Liderlik             int
	Sogukkanlilik        int
	Konsantrasyon        int
	TaktigeBaglilik      int
	Ceviklik             int
	Dayaniklilik         int
	Denge                int
	Guc                  int
	Hiz                  int
	Hizlanma             int
	Ziplama              int
	Refleks              int
	Degaj                int
	KarsiKarsiya         int
}

type Futbolcu struct {
	Isim       string
	Soyisim    string
	Mevki      string
	Yetenek    int
	Potansiyel int
	Profil     Ozellikler
	Boy        int
	Kilo       float64
}

type Olay struct {
	Aksiyon string
}

type takim struct {
	Kaleci      int
	OrtaSaha    int
	Defans      int
	Hucum       int
	OrtalamaGuc int
	GolSayisi   int
	TakimTaktik Taktik
	Kadro       []Futbolcu
}

func main() {

	/*kacanMesajlar := []string{"TOP DİREKTE PATLADI", "KALE AĞZINDAN DIŞARIYA VURDU", "BUNU NASIL KAÇIRIR?", "TAKIM ARKADAŞLARINDAN ÖZÜR DİLİYOR", "TOP DİREĞİN YANINDAN DIŞARIYA GİDİYOR", "AZ FARKLA AUT", "DAĞLARA TAŞLARA", "REZİL BİR ŞUT"}
	asistMesajlari := []string{"AKIL DOLU BİR PAS", "MÜTHİŞ SERVİS", "TEKTE OYNUYOR", "TOPUKLA BIRAKIYOR"}
	golMesajlari := []string{"MÜTHİŞ BİR GOL", "KALECİ TOPU İZLEMEKTEN BAŞKA HİÇBİR ŞEY YAPAMADI", "AĞLARI DELİYOR", "TAM DOKSANA", "ÖRÜMCEK AĞLARINI AVLADI", "FİLELERİ HAVALANDIRIYOR", "ONA SADECE DOKUNMAK KALIYOR"}*/

	var ilkTakim takim
	var ikinciTakim takim
	// var hucumSansFaktoru int
	// var defansSansFaktoru int
	var macRaporu []Olay
	var toplamDefansGucu int
	var toplamOrtaSahaGucu int
	var toplamHucumGucu int
	var takimAdi string
	var topaSahipOyuncu Futbolcu
	var seciliTaktik Taktik
	var topaSahipOyuncuIndex int
	defansOyuncuSayisi := 0
	ortaSahaOyuncuSayisi := 0
	hucumOyuncuSayisi := 0
	path, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("Kullanıcının home dizini bulunamadı.")
	}

	Ali := Futbolcu{
		Isim:    "Ali",
		Soyisim: "Yilmaz",
		Mevki:   "Kaleci",
		Yetenek: 170,
		Profil:  ProfilOlustur("Kaleci", 170),
	}
	Veli := Futbolcu{
		Isim:    "Veli",
		Soyisim: "Bakir",
		Mevki:   "SolBek",
		Yetenek: 160,
		Profil:  ProfilOlustur("SolBek", 160),
	}
	Mehmet := Futbolcu{
		Isim:    "Mehmet",
		Soyisim: "Gunes",
		Mevki:   "Stoper",
		Yetenek: 140,
		Profil:  ProfilOlustur("Stoper", 140),
	}
	Alperen := Futbolcu{
		Isim:    "Alperen",
		Soyisim: "Celik",
		Mevki:   "Stoper",
		Yetenek: 170,
		Profil:  ProfilOlustur("Stoper", 170),
	}
	Poyraz := Futbolcu{
		Isim:    "Poyraz",
		Soyisim: "Demir",
		Mevki:   "SagBek",
		Yetenek: 160,
		Profil:  ProfilOlustur("SagBek", 160),
	}
	Metehan := Futbolcu{
		Isim:    "Metehan",
		Soyisim: "Kaya",
		Mevki:   "OrtaSaha",
		Yetenek: 140,
		Profil:  ProfilOlustur("OrtaSaha", 140),
	}
	Cagan := Futbolcu{
		Isim:    "Cagan",
		Soyisim: "Yilmaz",
		Mevki:   "OrtaSaha",
		Yetenek: 170,
		Profil:  ProfilOlustur("OrtaSaha", 140),
	}
	Goktug := Futbolcu{
		Isim:    "Goktug",
		Soyisim: "Sahin",
		Mevki:   "OrtaSaha",
		Yetenek: 160,
		Profil:  ProfilOlustur("OrtaSaha", 160),
	}
	Aras := Futbolcu{
		Isim:    "Aras",
		Soyisim: "Aydin",
		Mevki:   "SolKanat",
		Yetenek: 140,
		Profil:  ProfilOlustur("SolKanat", 140),
	}
	Cinar := Futbolcu{
		Isim:    "Cinar",
		Soyisim: "Ozturk",
		Mevki:   "Forvet",
		Yetenek: 170,
		Profil:  ProfilOlustur("Forvet", 170),
	}
	Bora := Futbolcu{
		Isim:    "Bora",
		Soyisim: "Yildirim",
		Mevki:   "SagKanat",
		Yetenek: 160,
		Profil:  ProfilOlustur("SagKanat", 170),
	}

	ilkTakim.Kadro = append(ilkTakim.Kadro, Ali, Veli, Mehmet, Alperen, Poyraz, Metehan, Cagan, Goktug, Aras, Cinar, Bora)

	Atlas := Futbolcu{
		Isim:    "Atlas",
		Soyisim: "Koc",
		Mevki:   "Kaleci",
		Yetenek: 170,
		Profil:  ProfilOlustur("Kaleci", 170),
	}
	Kuzey := Futbolcu{
		Isim:    "Kuzey",
		Soyisim: "Cetin",
		Mevki:   "SolBek",
		Yetenek: 160,
		Profil:  ProfilOlustur("SolBek", 160),
	}
	Sarp := Futbolcu{
		Isim:    "Sarp",
		Soyisim: "Dogan",
		Mevki:   "Stoper",
		Yetenek: 140,
		Profil:  ProfilOlustur("Stoper", 140),
	}
	Doruk := Futbolcu{
		Isim:    "Doruk",
		Soyisim: "Sonmez",
		Mevki:   "Stoper",
		Yetenek: 170,
		Profil:  ProfilOlustur("Stoper", 170),
	}
	Eymen := Futbolcu{
		Isim:    "Eymen",
		Soyisim: "Keskin",
		Mevki:   "SagBek",
		Yetenek: 160,
		Profil:  ProfilOlustur("SagBek", 160),
	}
	Bartu := Futbolcu{
		Isim:    "Bartu",
		Soyisim: "Tekin",
		Mevki:   "OrtaSaha",
		Yetenek: 140,
		Profil:  ProfilOlustur("OrtaSaha", 140),
	}
	Yigit := Futbolcu{
		Isim:    "Yigit",
		Soyisim: "Arslan",
		Mevki:   "OrtaSaha",
		Yetenek: 170,
		Profil:  ProfilOlustur("OrtaSaha", 170),
	}
	Yagiz := Futbolcu{
		Isim:    "Yagiz",
		Soyisim: "Korkmaz",
		Mevki:   "OrtaSaha",
		Yetenek: 160,
		Profil:  ProfilOlustur("OrtaSaha", 160),
	}
	Deniz := Futbolcu{
		Isim:    "Deniz",
		Soyisim: "Aksoy",
		Mevki:   "SolKanat",
		Yetenek: 140,
		Profil:  ProfilOlustur("SolKanat", 140),
	}
	Efe := Futbolcu{
		Isim:    "Efe",
		Soyisim: "Ozkan",
		Mevki:   "Forvet",
		Yetenek: 170,
		Profil:  ProfilOlustur("Forvet", 170),
	}
	Utku := Futbolcu{
		Isim:    "Utku",
		Soyisim: "Erdem",
		Mevki:   "SagKanat",
		Yetenek: 160,
		Profil:  ProfilOlustur("SagKanat", 160),
	}

	ikinciTakim.Kadro = append(ikinciTakim.Kadro, Atlas, Kuzey, Sarp, Doruk, Eymen, Bartu, Yigit, Yagiz, Deniz, Efe, Utku)

	for i := range ilkTakim.Kadro {
		if ilkTakim.Kadro[i].Mevki == "Kaleci" {
			ilkTakim.Kaleci = (rand.Intn((ilkTakim.Kadro[i].Yetenek)/5) + 4)
		}
		if ilkTakim.Kadro[i].Mevki == "Stoper" || ilkTakim.Kadro[i].Mevki == "SolBek" || ilkTakim.Kadro[i].Mevki == "SagBek" {
			toplamDefansGucu += ilkTakim.Kadro[i].Yetenek
			defansOyuncuSayisi++
		}
		if ilkTakim.Kadro[i].Mevki == "OrtaSaha" {
			toplamOrtaSahaGucu += ilkTakim.Kadro[i].Yetenek
			ortaSahaOyuncuSayisi++
		}
		if ilkTakim.Kadro[i].Mevki == "Forvet" || ilkTakim.Kadro[i].Mevki == "SolKanat" || ilkTakim.Kadro[i].Mevki == "SagKanat" {
			toplamHucumGucu += ilkTakim.Kadro[i].Yetenek
			hucumOyuncuSayisi++
		}
	}
	ilkTakim.Defans = toplamDefansGucu / defansOyuncuSayisi
	ilkTakim.OrtaSaha = toplamOrtaSahaGucu / ortaSahaOyuncuSayisi
	ilkTakim.Hucum = toplamHucumGucu / hucumOyuncuSayisi
	toplamDefansGucu = 0
	toplamOrtaSahaGucu = 0
	toplamHucumGucu = 0
	defansOyuncuSayisi = 0
	ortaSahaOyuncuSayisi = 0
	hucumOyuncuSayisi = 0
	for i := range ikinciTakim.Kadro {
		if ikinciTakim.Kadro[i].Mevki == "Kaleci" {
			ikinciTakim.Kaleci = (rand.Intn((ikinciTakim.Kadro[i].Yetenek)/5) + 4)
		}
		if ikinciTakim.Kadro[i].Mevki == "Stoper" || ikinciTakim.Kadro[i].Mevki == "SolBek" || ikinciTakim.Kadro[i].Mevki == "SagBek" {
			toplamDefansGucu += ikinciTakim.Kadro[i].Yetenek
			defansOyuncuSayisi++
		}
		if ikinciTakim.Kadro[i].Mevki == "OrtaSaha" {
			toplamOrtaSahaGucu += ikinciTakim.Kadro[i].Yetenek
			ortaSahaOyuncuSayisi++
		}
		if ikinciTakim.Kadro[i].Mevki == "Forvet" || ikinciTakim.Kadro[i].Mevki == "SolKanat" || ikinciTakim.Kadro[i].Mevki == "SagKanat" {
			toplamHucumGucu += ikinciTakim.Kadro[i].Yetenek
			hucumOyuncuSayisi++
		}
	}
	ikinciTakim.Defans = toplamDefansGucu / defansOyuncuSayisi
	ikinciTakim.OrtaSaha = toplamOrtaSahaGucu / ortaSahaOyuncuSayisi
	ikinciTakim.Hucum = toplamHucumGucu / hucumOyuncuSayisi
	toplamDefansGucu = 0
	toplamOrtaSahaGucu = 0
	toplamHucumGucu = 0
	defansOyuncuSayisi = 0
	ortaSahaOyuncuSayisi = 0
	hucumOyuncuSayisi = 0

	ilkTakim.OrtalamaGuc = (ilkTakim.OrtaSaha + ilkTakim.Defans + ilkTakim.Hucum) / 3
	ikinciTakim.OrtalamaGuc = (ikinciTakim.Defans + ikinciTakim.Hucum + ikinciTakim.OrtaSaha) / 3
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

	anlikBolge := 2
	isimIndex := rand.Intn(2)
	if isimIndex == 0 {
		takimAdi = "ilkTakim"
		topaSahipOyuncuIndex = 6
		topaSahipOyuncu = ilkTakim.Kadro[topaSahipOyuncuIndex]
		seciliTaktik = ilkTakim.TakimTaktik
	} else {
		takimAdi = "ikinciTakim"
		topaSahipOyuncuIndex = 6
		topaSahipOyuncu = ikinciTakim.Kadro[topaSahipOyuncuIndex]
		seciliTaktik = ikinciTakim.TakimTaktik
	}

	for i := 0; i <= 90; i++ {

		aksiyon := KararVer(topaSahipOyuncu, anlikBolge, seciliTaktik)

		if aksiyon == "Kısa Pas" {
			hedefOyuncuIndex := rand.Intn(11)
			for {
				if hedefOyuncuIndex == topaSahipOyuncuIndex || hedefOyuncuIndex > 10 || hedefOyuncuIndex < 0 {
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
						anlikBolge -= 1
					} else {
						anlikBolge += 1
					}
					if anlikBolge > 3 {
						anlikBolge = 3
					} else if anlikBolge < 1 {
						anlikBolge = 1
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s boştaki %s'e kısa oynuyor", i, topaSahipOyuncu.Isim, hedefOyuncu.Isim),
					}
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = hedefOyuncu
					topaSahipOyuncuIndex = hedefOyuncuIndex
				} else {
					hedefOyuncu := ikinciTakim.Kadro[hedefOyuncuIndex]

					geriPasIhtimali := 35
					yanPasIhtimali := 45
					ileriPasIhtimali := 20
					pasIhtimalleri := geriPasIhtimali + yanPasIhtimali + ileriPasIhtimali
					pasZar := rand.Intn(pasIhtimalleri)
					if pasZar < yanPasIhtimali {
					} else if pasZar < (yanPasIhtimali + geriPasIhtimali) {
						anlikBolge -= 1
					} else {
						anlikBolge += 1
					}
					if anlikBolge > 3 {
						anlikBolge = 3
					} else if anlikBolge < 1 {
						anlikBolge = 1
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s boştaki %s'e kısa oynuyor", i, topaSahipOyuncu.Isim, hedefOyuncu.Isim),
					}
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = hedefOyuncu
					topaSahipOyuncuIndex = hedefOyuncuIndex
				}
			} else {
				if takimAdi == "ilkTakim" {
					takimAdi = "ikinciTakim"
					seciliTaktik = ikinciTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s pas hatası! %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					bolgeCevirici := 4 - anlikBolge
					anlikBolge = int(bolgeCevirici)
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipOyuncu
					topaSahipOyuncuIndex = rakipOyuncuIndex
				} else {
					takimAdi = "ilkTakim"
					seciliTaktik = ilkTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s pas hatası! %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					bolgeCevirici := 4 - anlikBolge
					anlikBolge = int(bolgeCevirici)
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipOyuncu
					topaSahipOyuncuIndex = rakipOyuncuIndex
				}
			}
		} else if aksiyon == "Uzun Pas" || aksiyon == "Degaj" {
			hedefOyuncuIndex := rand.Intn(10) + 1
			for {
				if hedefOyuncuIndex == topaSahipOyuncuIndex || hedefOyuncuIndex > 10 || hedefOyuncuIndex < 0 {
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
					anlikBolge += 1
					if anlikBolge > 3 {
						anlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s boştaki %s'e uzun bir top", i, topaSahipOyuncu.Isim, hedefOyuncu.Isim),
					}
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = hedefOyuncu
					topaSahipOyuncuIndex = hedefOyuncuIndex
				} else {
					hedefOyuncu := ikinciTakim.Kadro[hedefOyuncuIndex]
					anlikBolge += 1
					if anlikBolge > 3 {
						anlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s boştaki %s'e uzun bir top", i, topaSahipOyuncu.Isim, hedefOyuncu.Isim),
					}
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = hedefOyuncu
					topaSahipOyuncuIndex = hedefOyuncuIndex
				}
			} else {
				if takimAdi == "ilkTakim" {
					takimAdi = "ikinciTakim"
					seciliTaktik = ikinciTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s pas hatası! %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					bolgeCevirici := 4 - anlikBolge
					anlikBolge = int(bolgeCevirici)
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipOyuncu
					topaSahipOyuncuIndex = rakipOyuncuIndex
				} else {
					takimAdi = "ilkTakim"
					seciliTaktik = ilkTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s pas hatası! %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					bolgeCevirici := 4 - anlikBolge
					anlikBolge = int(bolgeCevirici)
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipOyuncu
					topaSahipOyuncuIndex = rakipOyuncuIndex
				}
			}
		} else if aksiyon == "Erken Orta" || aksiyon == "Orta" {
			hedefOyuncuIndex := rand.Intn(10) + 1
			for {
				if hedefOyuncuIndex == topaSahipOyuncuIndex || hedefOyuncuIndex > 10 || hedefOyuncuIndex < 0 {
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
					anlikBolge += 1
					if anlikBolge > 3 {
						anlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s'den içerideki %s'e güzel bir orta", i, topaSahipOyuncu.Isim, hedefOyuncu.Isim),
					}
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = hedefOyuncu
					topaSahipOyuncuIndex = hedefOyuncuIndex
				} else {
					hedefOyuncu := ikinciTakim.Kadro[hedefOyuncuIndex]
					anlikBolge += 1
					if anlikBolge > 3 {
						anlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s'den içerideki %s'e güzel bir orta", i, topaSahipOyuncu.Isim, hedefOyuncu.Isim),
					}
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = hedefOyuncu
					topaSahipOyuncuIndex = hedefOyuncuIndex
				}
			} else {
				if takimAdi == "ilkTakim" {
					takimAdi = "ikinciTakim"
					seciliTaktik = ikinciTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s'den rezalet bir orta! %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					bolgeCevirici := 4 - anlikBolge
					anlikBolge = int(bolgeCevirici)
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipOyuncu
					topaSahipOyuncuIndex = rakipOyuncuIndex
				} else {
					takimAdi = "ilkTakim"
					seciliTaktik = ilkTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s'den rezalet bir orta! %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					bolgeCevirici := 4 - anlikBolge
					anlikBolge = int(bolgeCevirici)
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipOyuncu
					topaSahipOyuncuIndex = rakipOyuncuIndex
				}
			}
		} else if aksiyon == "Dikine Pas" || aksiyon == "Kilit Pas" {
			hedefOyuncuIndex := rand.Intn(10) + 1
			for {
				if hedefOyuncuIndex == topaSahipOyuncuIndex || hedefOyuncuIndex > 10 || hedefOyuncuIndex < 0 {
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
					anlikBolge += 1
					if anlikBolge > 3 {
						anlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s'den ilerideki %s'e müthiş bir pas.", i, topaSahipOyuncu.Isim, hedefOyuncu.Isim),
					}
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = hedefOyuncu
					topaSahipOyuncuIndex = hedefOyuncuIndex
				} else {
					hedefOyuncu := ikinciTakim.Kadro[hedefOyuncuIndex]
					anlikBolge += 1
					if anlikBolge > 3 {
						anlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s'den ilerideki %s'e müthiş bir pas.", i, topaSahipOyuncu.Isim, hedefOyuncu.Isim),
					}
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = hedefOyuncu
					topaSahipOyuncuIndex = hedefOyuncuIndex
				}
			} else {
				if takimAdi == "ilkTakim" {
					takimAdi = "ikinciTakim"
					seciliTaktik = ikinciTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s arayı iyi gördü! Fakat %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					bolgeCevirici := 4 - anlikBolge
					anlikBolge = int(bolgeCevirici)
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipOyuncu
					topaSahipOyuncuIndex = rakipOyuncuIndex
				} else {
					takimAdi = "ilkTakim"
					seciliTaktik = ilkTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s arayı iyi gördü! Fakat %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					bolgeCevirici := 4 - anlikBolge
					anlikBolge = int(bolgeCevirici)
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipOyuncu
					topaSahipOyuncuIndex = rakipOyuncuIndex
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
					anlikBolge += 1
					if anlikBolge > 3 {
						anlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s'den nefis bir çalım %s dondu kaldı", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					macRaporu = append(macRaporu, yasananAksiyon)
				} else {
					anlikBolge += 1
					if anlikBolge > 3 {
						anlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s'den nefis bir çalım %s dondu kaldı", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					macRaporu = append(macRaporu, yasananAksiyon)
				}
			} else {
				if takimAdi == "ilkTakim" {
					takimAdi = "ikinciTakim"
					seciliTaktik = ikinciTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s'den çalım denemesi.. Fakat %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					bolgeCevirici := 4 - anlikBolge
					anlikBolge = int(bolgeCevirici)
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipOyuncu
					topaSahipOyuncuIndex = rakipOyuncuIndex
				} else {
					takimAdi = "ilkTakim"
					seciliTaktik = ilkTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s'den çalım denemesi.. Fakat %s topu kapıyor", i, topaSahipOyuncu.Isim, rakipOyuncu.Isim),
					}
					bolgeCevirici := 4 - anlikBolge
					anlikBolge = int(bolgeCevirici)
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipOyuncu
					topaSahipOyuncuIndex = rakipOyuncuIndex
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
					anlikBolge += 1
					if anlikBolge > 3 {
						anlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s Uzaklardan bir şut.. TOP AĞLARDA GOOOOOOOOOOOL %s dondu kaldı", i, topaSahipOyuncu.Isim, rakipKaleci.Isim),
					}
					ilkTakim.GolSayisi++
					topaSahipOyuncu = ikinciTakim.Kadro[6]
					topaSahipOyuncuIndex = 6
					anlikBolge = 2
					macRaporu = append(macRaporu, yasananAksiyon)
				} else {
					takimAdi = "ilkTakim"
					seciliTaktik = ilkTakim.TakimTaktik
					anlikBolge += 1
					if anlikBolge > 3 {
						anlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s Uzaklardan bir şut.. TOP AĞLARDA GOOOOOOOOOOOL %s dondu kaldı", i, topaSahipOyuncu.Isim, rakipKaleci.Isim),
					}
					ikinciTakim.GolSayisi++
					topaSahipOyuncu = ilkTakim.Kadro[6]
					topaSahipOyuncuIndex = 6
					anlikBolge = 2
					macRaporu = append(macRaporu, yasananAksiyon)
				}
			} else {
				if takimAdi == "ilkTakim" {
					takimAdi = "ikinciTakim"
					seciliTaktik = ikinciTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s Uzaklardan bir şut.. Fakat %s topu rahatça alıyor", i, topaSahipOyuncu.Isim, rakipKaleci.Isim),
					}
					bolgeCevirici := 4 - anlikBolge
					anlikBolge = int(bolgeCevirici)
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipKaleci
					topaSahipOyuncuIndex = 0
				} else {
					takimAdi = "ilkTakim"
					seciliTaktik = ilkTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s Uzaklardan bir şut.. Fakat %s topu rahatça alıyor", i, topaSahipOyuncu.Isim, rakipKaleci.Isim),
					}
					bolgeCevirici := 4 - anlikBolge
					anlikBolge = int(bolgeCevirici)
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipKaleci
					topaSahipOyuncuIndex = 0
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
					anlikBolge += 1
					if anlikBolge > 3 {
						anlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s Güzel bir şut.. TOP AĞLARDA GOOOOOOOOOOOL %s'nin yapabilecek hiçbir şeyi yok", i, topaSahipOyuncu.Isim, rakipKaleci.Isim),
					}
					ilkTakim.GolSayisi++
					topaSahipOyuncu = ikinciTakim.Kadro[6]
					topaSahipOyuncuIndex = 6
					anlikBolge = 2
					macRaporu = append(macRaporu, yasananAksiyon)
				} else {
					takimAdi = "ilkTakim"
					seciliTaktik = ilkTakim.TakimTaktik
					anlikBolge += 1
					if anlikBolge > 3 {
						anlikBolge = 3
					}
					yasananAksiyon := Olay{
						Aksiyon: fmt.Sprintf("%d. DAKİKA: %s Güzel bir şut.. TOP AĞLARDA GOOOOOOOOOOOL %s'nin yapabilecek hiçbir şeyi yok", i, topaSahipOyuncu.Isim, rakipKaleci.Isim),
					}
					ikinciTakim.GolSayisi++
					topaSahipOyuncu = ilkTakim.Kadro[6]
					topaSahipOyuncuIndex = 6
					anlikBolge = 2
					macRaporu = append(macRaporu, yasananAksiyon)
				}
			} else {
				if takimAdi == "ilkTakim" {
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
					} else if basarisizlikZari < (autIhtimali + kornerIhtimali) {
						metin = fmt.Sprintf("%d. DAKİKA: %s'den nefis bir şut %s aynı güzellikle bir kurtarışa imza atıyor ", i, topaSahipOyuncu.Isim, rakipKaleci.Isim)
					} else if basarisizlikZari < (autIhtimali + kornerIhtimali + kalecideKalmaIhtimali) {
						metin = fmt.Sprintf("%d. DAKİKA: %s Bir şut.. Fakat %s topu rahatça alıyor", i, topaSahipOyuncu.Isim, rakipKaleci.Isim)
					} else if basarisizlikZari < (autIhtimali + kornerIhtimali + kalecideKalmaIhtimali + kalecidenDonmeIhtimali) {
						metin = fmt.Sprintf("%d. DAKİKA: %s'den isabetli bir şut %s'den seken top boşta kaldı!", i, topaSahipOyuncu.Isim, rakipKaleci.Isim)
					} else {
						metin = fmt.Sprintf("%d. DAKİKA: %s Bir şut.. TOP DİREKTE PATLIYOR", i, topaSahipOyuncu.Isim)
					}
					takimAdi = "ikinciTakim"
					seciliTaktik = ikinciTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: metin,
					}
					bolgeCevirici := 4 - anlikBolge
					anlikBolge = int(bolgeCevirici)
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipKaleci
					topaSahipOyuncuIndex = 0
				} else {
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
					} else if basarisizlikZari < (autIhtimali + kornerIhtimali) {
						metin = fmt.Sprintf("%d. DAKİKA: %s'den nefis bir şut %s aynı güzellikle bir kurtarışa imza atıyor ", i, topaSahipOyuncu.Isim, rakipKaleci.Isim)
					} else if basarisizlikZari < (autIhtimali + kornerIhtimali + kalecideKalmaIhtimali) {
						metin = fmt.Sprintf("%d. DAKİKA: %s Bir şut.. Fakat %s topu rahatça alıyor", i, topaSahipOyuncu.Isim, rakipKaleci.Isim)
					} else if basarisizlikZari < (autIhtimali + kornerIhtimali + kalecideKalmaIhtimali + kalecidenDonmeIhtimali) {
						metin = fmt.Sprintf("%d. DAKİKA: %s'den isabetli bir şut %s'den seken top boşta kaldı!", i, topaSahipOyuncu.Isim, rakipKaleci.Isim)
					} else {
						metin = fmt.Sprintf("%d. DAKİKA: %s Bir şut.. TOP DİREKTE PATLIYOR", i, topaSahipOyuncu.Isim)
					}
					takimAdi = "ilkTakim"
					seciliTaktik = ilkTakim.TakimTaktik
					yasananAksiyon := Olay{
						Aksiyon: metin,
					}
					bolgeCevirici := 4 - anlikBolge
					anlikBolge = int(bolgeCevirici)
					macRaporu = append(macRaporu, yasananAksiyon)
					topaSahipOyuncu = rakipKaleci
					topaSahipOyuncuIndex = 0
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
			macRaporu = append(macRaporu, yasananAksiyon)
		}
	}
	dondurulenListe, err := json.Marshal(macRaporu)
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

func DuranTop() {
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

func ProfilOlustur(mevki string, yetenek int) Ozellikler {
	var player Ozellikler
	temelPuan := yetenek / 10

	if mevki == "Kaleci" {
		player.Degaj = temelPuan + rand.Intn(4) - 1
		player.KarsiKarsiya = temelPuan + rand.Intn(4) - 1
		player.Refleks = temelPuan + rand.Intn(4) - 1
		player.Ziplama = temelPuan + rand.Intn(4) - 1
		player.Bitiricilik = rand.Intn(8) + 1
		player.Caliskanlik = rand.Intn(8) + 1
		player.Cesaret = rand.Intn(8) + 1
		player.Ceviklik = rand.Intn(8) + 1
		player.Dayaniklilik = rand.Intn(8) + 1
		player.DefansifPozisyonAlma = rand.Intn(8) + 1
		player.Denge = rand.Intn(8) + 1
		player.Dribling = rand.Intn(8) + 1
		player.DuranToplar = rand.Intn(8) + 1
		player.Guc = rand.Intn(8) + 1
		player.Hiz = rand.Intn(8) + 1
		player.Hizlanma = rand.Intn(8) + 1
		player.IlkKontrol = rand.Intn(8) + 1
		player.KafaVurusu = rand.Intn(8) + 1
		player.KararAlma = rand.Intn(8) + 1
		player.Kararlilik = rand.Intn(8) + 1
		player.Konsantrasyon = rand.Intn(8) + 1
		player.Liderlik = rand.Intn(8) + 1
		player.Markaj = rand.Intn(8) + 1
		player.OnSezi = rand.Intn(8) + 1
		player.OrtaYapma = rand.Intn(8) + 1
		player.Pas = rand.Intn(8) + 1
		player.Sogukkanlilik = rand.Intn(8) + 1
		player.TaktigeBaglilik = rand.Intn(8) + 1
		player.Teknik = rand.Intn(8) + 1
		player.TopKapma = rand.Intn(8) + 1
		player.TopsuzAlan = rand.Intn(8) + 1
		player.UzaktanSut = rand.Intn(8) + 1
		player.Vizyon = rand.Intn(8) + 1
	} else if mevki == "Stoper" {
		player.TopKapma = temelPuan + rand.Intn(4) - 1
		player.Markaj = temelPuan + rand.Intn(4) - 1
		player.DefansifPozisyonAlma = temelPuan + rand.Intn(4) - 1
		player.Ziplama = temelPuan + rand.Intn(4) - 1
		player.Degaj = rand.Intn(8) + 1
		player.KarsiKarsiya = rand.Intn(8) + 1
		player.Refleks = rand.Intn(8) + 1
		player.Bitiricilik = rand.Intn(8) + 1
		player.Caliskanlik = rand.Intn(8) + 1
		player.Cesaret = rand.Intn(8) + 1
		player.Ceviklik = rand.Intn(8) + 1
		player.Dayaniklilik = rand.Intn(8) + 1
		player.Denge = rand.Intn(8) + 1
		player.Dribling = rand.Intn(8) + 1
		player.DuranToplar = rand.Intn(8) + 1
		player.Guc = rand.Intn(8) + 1
		player.Hiz = rand.Intn(8) + 1
		player.Hizlanma = rand.Intn(8) + 1
		player.IlkKontrol = rand.Intn(8) + 1
		player.KafaVurusu = rand.Intn(8) + 1
		player.KararAlma = rand.Intn(8) + 1
		player.Kararlilik = rand.Intn(8) + 1
		player.Konsantrasyon = rand.Intn(8) + 1
		player.Liderlik = rand.Intn(8) + 1
		player.OnSezi = rand.Intn(8) + 1
		player.OrtaYapma = rand.Intn(8) + 1
		player.Pas = rand.Intn(8) + 1
		player.Sogukkanlilik = rand.Intn(8) + 1
		player.TaktigeBaglilik = rand.Intn(8) + 1
		player.Teknik = rand.Intn(8) + 1
		player.TopsuzAlan = rand.Intn(8) + 1
		player.UzaktanSut = rand.Intn(8) + 1
		player.Vizyon = rand.Intn(8) + 1
	} else if mevki == "SolBek" || mevki == "SagBek" {
		player.TopKapma = temelPuan + rand.Intn(4) - 1
		player.Markaj = temelPuan + rand.Intn(4) - 1
		player.DefansifPozisyonAlma = temelPuan + rand.Intn(4) - 1
		player.Hiz = temelPuan + rand.Intn(4) - 1
		player.Hizlanma = temelPuan + rand.Intn(4) - 1
		player.OrtaYapma = temelPuan + rand.Intn(4) - 1
		player.Pas = temelPuan + rand.Intn(4) - 1
		player.Teknik = temelPuan + rand.Intn(4) - 1
		player.Dribling = temelPuan + rand.Intn(4) - 1
		player.Ziplama = rand.Intn(8) + 1
		player.Degaj = rand.Intn(8) + 1
		player.KarsiKarsiya = rand.Intn(8) + 1
		player.Refleks = rand.Intn(8) + 1
		player.Bitiricilik = rand.Intn(8) + 1
		player.Caliskanlik = rand.Intn(8) + 1
		player.Cesaret = rand.Intn(8) + 1
		player.Ceviklik = rand.Intn(8) + 1
		player.Dayaniklilik = rand.Intn(8) + 1
		player.Denge = rand.Intn(8) + 1
		player.DuranToplar = rand.Intn(8) + 1
		player.Guc = rand.Intn(8) + 1
		player.IlkKontrol = rand.Intn(8) + 1
		player.KafaVurusu = rand.Intn(8) + 1
		player.KararAlma = rand.Intn(8) + 1
		player.Kararlilik = rand.Intn(8) + 1
		player.Konsantrasyon = rand.Intn(8) + 1
		player.Liderlik = rand.Intn(8) + 1
		player.OnSezi = rand.Intn(8) + 1
		player.Sogukkanlilik = rand.Intn(8) + 1
		player.TaktigeBaglilik = rand.Intn(8) + 1
		player.TopsuzAlan = rand.Intn(8) + 1
		player.UzaktanSut = rand.Intn(8) + 1
		player.Vizyon = rand.Intn(8) + 1
	} else if mevki == "SolKanat" || mevki == "SagKanat" {
		player.Hiz = temelPuan + rand.Intn(4) - 1
		player.Hizlanma = temelPuan + rand.Intn(4) - 1
		player.OrtaYapma = temelPuan + rand.Intn(4) - 1
		player.Pas = temelPuan + rand.Intn(4) - 1
		player.Teknik = temelPuan + rand.Intn(4) - 1
		player.Dribling = temelPuan + rand.Intn(4) - 1
		player.UzaktanSut = temelPuan + rand.Intn(4) - 1
		player.Vizyon = temelPuan + rand.Intn(4) - 1
		player.Bitiricilik = temelPuan + rand.Intn(4) - 1
		player.Ziplama = rand.Intn(8) + 1
		player.TopKapma = rand.Intn(8) + 1
		player.Markaj = rand.Intn(8) + 1
		player.DefansifPozisyonAlma = rand.Intn(8) + 1
		player.Degaj = rand.Intn(8) + 1
		player.KarsiKarsiya = rand.Intn(8) + 1
		player.Refleks = rand.Intn(8) + 1
		player.Caliskanlik = rand.Intn(8) + 1
		player.Cesaret = rand.Intn(8) + 1
		player.Ceviklik = rand.Intn(8) + 1
		player.Dayaniklilik = rand.Intn(8) + 1
		player.Denge = rand.Intn(8) + 1
		player.DuranToplar = rand.Intn(8) + 1
		player.Guc = rand.Intn(8) + 1
		player.IlkKontrol = rand.Intn(8) + 1
		player.KafaVurusu = rand.Intn(8) + 1
		player.KararAlma = rand.Intn(8) + 1
		player.Kararlilik = rand.Intn(8) + 1
		player.Konsantrasyon = rand.Intn(8) + 1
		player.Liderlik = rand.Intn(8) + 1
		player.OnSezi = rand.Intn(8) + 1
		player.Sogukkanlilik = rand.Intn(8) + 1
		player.TaktigeBaglilik = rand.Intn(8) + 1
		player.TopsuzAlan = rand.Intn(8) + 1
	} else if mevki == "DefansifOrtaSaha" {
		player.OrtaYapma = temelPuan + rand.Intn(4) - 1
		player.Pas = temelPuan + rand.Intn(4) - 1
		player.Teknik = temelPuan + rand.Intn(4) - 1
		player.TopKapma = temelPuan + rand.Intn(4) - 1
		player.Markaj = temelPuan + rand.Intn(4) - 1
		player.DefansifPozisyonAlma = temelPuan + rand.Intn(4) - 1
		player.UzaktanSut = temelPuan + rand.Intn(4) - 1
		player.Vizyon = temelPuan + rand.Intn(4) - 1
		player.IlkKontrol = temelPuan + rand.Intn(4) - 1
		player.Bitiricilik = rand.Intn(8) + 1
		player.Dayaniklilik = rand.Intn(8) + 1
		player.Ziplama = rand.Intn(8) + 1
		player.Hiz = rand.Intn(8) + 1
		player.Dribling = rand.Intn(8) + 1
		player.Hizlanma = rand.Intn(8) + 1
		player.Degaj = rand.Intn(8) + 1
		player.KarsiKarsiya = rand.Intn(8) + 1
		player.Refleks = rand.Intn(8) + 1
		player.Caliskanlik = rand.Intn(8) + 1
		player.Cesaret = rand.Intn(8) + 1
		player.Ceviklik = rand.Intn(8) + 1
		player.Denge = rand.Intn(8) + 1
		player.DuranToplar = rand.Intn(8) + 1
		player.Guc = rand.Intn(8) + 1
		player.KafaVurusu = rand.Intn(8) + 1
		player.KararAlma = rand.Intn(8) + 1
		player.Kararlilik = rand.Intn(8) + 1
		player.Konsantrasyon = rand.Intn(8) + 1
		player.Liderlik = rand.Intn(8) + 1
		player.OnSezi = rand.Intn(8) + 1
		player.Sogukkanlilik = rand.Intn(8) + 1
		player.TaktigeBaglilik = rand.Intn(8) + 1
		player.TopsuzAlan = rand.Intn(8) + 1
	} else if mevki == "OrtaSaha" {
		player.OrtaYapma = temelPuan + rand.Intn(4) - 1
		player.Pas = temelPuan + rand.Intn(4) - 1
		player.Teknik = temelPuan + rand.Intn(4) - 1
		player.UzaktanSut = temelPuan + rand.Intn(4) - 1
		player.Vizyon = temelPuan + rand.Intn(4) - 1
		player.Dayaniklilik = temelPuan + rand.Intn(4) - 1
		player.IlkKontrol = temelPuan + rand.Intn(4) - 1
		player.Bitiricilik = rand.Intn(10) + 1
		player.Dribling = rand.Intn(12) + 1
		player.TopKapma = rand.Intn(12) + 1
		player.Markaj = rand.Intn(12) + 1
		player.DefansifPozisyonAlma = rand.Intn(12) + 1
		player.Ziplama = rand.Intn(8) + 1
		player.Hiz = rand.Intn(8) + 1
		player.Hizlanma = rand.Intn(8) + 1
		player.Degaj = rand.Intn(8) + 1
		player.KarsiKarsiya = rand.Intn(8) + 1
		player.Refleks = rand.Intn(8) + 1
		player.Caliskanlik = rand.Intn(8) + 1
		player.Cesaret = rand.Intn(8) + 1
		player.Ceviklik = rand.Intn(8) + 1
		player.Denge = rand.Intn(8) + 1
		player.DuranToplar = rand.Intn(8) + 1
		player.Guc = rand.Intn(8) + 1
		player.KafaVurusu = rand.Intn(8) + 1
		player.KararAlma = rand.Intn(8) + 1
		player.Kararlilik = rand.Intn(8) + 1
		player.Konsantrasyon = rand.Intn(8) + 1
		player.Liderlik = rand.Intn(8) + 1
		player.OnSezi = rand.Intn(8) + 1
		player.Sogukkanlilik = rand.Intn(8) + 1
		player.TaktigeBaglilik = rand.Intn(8) + 1
		player.TopsuzAlan = rand.Intn(8) + 1
	} else if mevki == "OfansifOrtaSaha" {
		player.OrtaYapma = temelPuan + rand.Intn(4) - 1
		player.Pas = temelPuan + rand.Intn(4) - 1
		player.Teknik = temelPuan + rand.Intn(4) - 1
		player.UzaktanSut = temelPuan + rand.Intn(4) - 1
		player.Vizyon = temelPuan + rand.Intn(4) - 1
		player.IlkKontrol = temelPuan + rand.Intn(4) - 1
		player.Bitiricilik = temelPuan + rand.Intn(4) - 1
		player.Dribling = temelPuan + rand.Intn(4) - 1
		player.Dayaniklilik = rand.Intn(12) + 1
		player.Ceviklik = rand.Intn(12) + 1
		player.Hiz = rand.Intn(12) + 1
		player.Hizlanma = rand.Intn(12) + 1
		player.TopKapma = rand.Intn(8) + 1
		player.Markaj = rand.Intn(8) + 1
		player.DefansifPozisyonAlma = rand.Intn(8) + 1
		player.Ziplama = rand.Intn(8) + 1
		player.Degaj = rand.Intn(8) + 1
		player.KarsiKarsiya = rand.Intn(8) + 1
		player.Refleks = rand.Intn(8) + 1
		player.Caliskanlik = rand.Intn(8) + 1
		player.Cesaret = rand.Intn(8) + 1
		player.Denge = rand.Intn(8) + 1
		player.DuranToplar = rand.Intn(8) + 1
		player.Guc = rand.Intn(8) + 1
		player.KafaVurusu = rand.Intn(8) + 1
		player.KararAlma = rand.Intn(8) + 1
		player.Kararlilik = rand.Intn(8) + 1
		player.Konsantrasyon = rand.Intn(8) + 1
		player.Liderlik = rand.Intn(8) + 1
		player.OnSezi = rand.Intn(8) + 1
		player.Sogukkanlilik = rand.Intn(8) + 1
		player.TaktigeBaglilik = rand.Intn(8) + 1
		player.TopsuzAlan = rand.Intn(8) + 1
	} else if mevki == "Forvet" {
		player.UzaktanSut = temelPuan + rand.Intn(4) - 1
		player.IlkKontrol = temelPuan + rand.Intn(4) - 1
		player.Bitiricilik = temelPuan + rand.Intn(4) - 1
		player.KafaVurusu = rand.Intn(8) + 1
		player.Dayaniklilik = rand.Intn(12) + 1
		player.Dribling = rand.Intn(12) + 1
		player.Ceviklik = rand.Intn(12) + 1
		player.OrtaYapma = rand.Intn(12) + 1
		player.Pas = rand.Intn(12) + 1
		player.Teknik = rand.Intn(12) + 1
		player.Vizyon = rand.Intn(12) + 1
		player.Hiz = rand.Intn(12) + 1
		player.OnSezi = rand.Intn(12) + 1
		player.Sogukkanlilik = rand.Intn(12) + 1
		player.Hizlanma = rand.Intn(12) + 1
		player.TopKapma = rand.Intn(8) + 1
		player.Markaj = rand.Intn(8) + 1
		player.DefansifPozisyonAlma = rand.Intn(8) + 1
		player.Ziplama = rand.Intn(8) + 1
		player.Degaj = rand.Intn(8) + 1
		player.KarsiKarsiya = rand.Intn(8) + 1
		player.Refleks = rand.Intn(8) + 1
		player.Caliskanlik = rand.Intn(8) + 1
		player.Cesaret = rand.Intn(8) + 1
		player.Denge = rand.Intn(8) + 1
		player.DuranToplar = rand.Intn(8) + 1
		player.Guc = rand.Intn(8) + 1
		player.KararAlma = rand.Intn(8) + 1
		player.Kararlilik = rand.Intn(8) + 1
		player.Konsantrasyon = rand.Intn(8) + 1
		player.Liderlik = rand.Intn(8) + 1
		player.TaktigeBaglilik = rand.Intn(8) + 1
		player.TopsuzAlan = rand.Intn(8) + 1
	}
	player.Bitiricilik = sinirla(player.Bitiricilik)
	player.UzaktanSut = sinirla(player.UzaktanSut)
	player.OnSezi = sinirla(player.OnSezi)
	player.Dribling = sinirla(player.Dribling)
	player.DuranToplar = sinirla(player.DuranToplar)
	player.Pas = sinirla(player.Pas)
	player.OrtaYapma = sinirla(player.OrtaYapma)
	player.Teknik = sinirla(player.Teknik)
	player.IlkKontrol = sinirla(player.IlkKontrol)
	player.Vizyon = sinirla(player.Vizyon)
	player.Markaj = sinirla(player.Markaj)
	player.TopKapma = sinirla(player.TopKapma)
	player.KafaVurusu = sinirla(player.KafaVurusu)
	player.DefansifPozisyonAlma = sinirla(player.DefansifPozisyonAlma)
	player.KararAlma = sinirla(player.KararAlma)
	player.TopsuzAlan = sinirla(player.TopsuzAlan)
	player.Caliskanlik = sinirla(player.Caliskanlik)
	player.Kararlilik = sinirla(player.Kararlilik)
	player.Cesaret = sinirla(player.Cesaret)
	player.Liderlik = sinirla(player.Liderlik)
	player.Sogukkanlilik = sinirla(player.Sogukkanlilik)
	player.Konsantrasyon = sinirla(player.Konsantrasyon)
	player.TaktigeBaglilik = sinirla(player.TaktigeBaglilik)
	player.Ceviklik = sinirla(player.Ceviklik)
	player.Dayaniklilik = sinirla(player.Dayaniklilik)
	player.Denge = sinirla(player.Denge)
	player.Guc = sinirla(player.Guc)
	player.Hiz = sinirla(player.Hiz)
	player.Hizlanma = sinirla(player.Hizlanma)
	player.Ziplama = sinirla(player.Ziplama)
	player.Refleks = sinirla(player.Refleks)
	player.Degaj = sinirla(player.Degaj)
	player.KarsiKarsiya = sinirla(player.KarsiKarsiya)

	return player
}
