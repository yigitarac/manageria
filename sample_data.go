package main

func OrnekIkiTakimOlustur() (ornekIlkTakim takim, ornekIkinciTakim takim) {

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

	ornekIlkTakim.Kadro = append(ornekIlkTakim.Kadro, Ali, Veli, Mehmet, Alperen, Poyraz, Metehan, Cagan, Goktug, Aras, Cinar, Bora)

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

	ornekIkinciTakim.Kadro = append(ornekIkinciTakim.Kadro, Atlas, Kuzey, Sarp, Doruk, Eymen, Bartu, Yigit, Yagiz, Deniz, Efe, Utku)

	return ornekIlkTakim, ornekIkinciTakim

}
