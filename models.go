package main

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
	Isim                  string
	Soyisim               string
	Mevki                 string
	Yetenek               int
	Potansiyel            int
	Profil                Ozellikler
	Boy                   int
	Kilo                  float64
	MactakiSariKartSayisi int
	MactaKirmizisiVarMi   bool
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
