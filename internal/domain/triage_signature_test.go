package domain

import "testing"

func TestNormalizeSenderEmail(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "bare uppercase", raw: "  BCA@BCA.CO.ID ", want: "bca@bca.co.id"},
		{name: "display name angle bracket", raw: "BCA <BCA@BCA.CO.ID>", want: "bca@bca.co.id"},
		{name: "quoted display name", raw: `"Grab Receipts" <No-Reply@Grab.com>`, want: "no-reply@grab.com"},
		{name: "empty", raw: "   ", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeSenderEmail(tc.raw); got != tc.want {
				t.Fatalf("NormalizeSenderEmail(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestDeriveEmailTemplateSignature_ObservedFixturesAndEdgeCases(t *testing.T) {
	bcaQris1 := DeriveEmailTemplateSignature(
		"BCA <BCA@BCA.CO.ID>",
		"Internet Transaction Journal",
		"Hello MUHAMMAD FARIZ,\nYou just made a transaction through myBCA.\nHere are the details of your transaction :\nStatus\n:\nSuccessful\nTransaction Date\n:\n24 Nov 2026 19:42:34\nTransaction Type\n:\nQRIS Payment",
	)
	bcaQris2 := DeriveEmailTemplateSignature(
		"bca@bca.co.id",
		"Internet Transaction Journal",
		"Hello MUHAMMAD FARIZ,\nYou just made a transaction through myBCA.\nHere are the details of your transaction :\nStatus\n:\nSuccessful\nTransaction Date\n:\n24 Nov 2026 19:38:41\nTransaction Type\n:\nQRIS Payment",
	)
	if bcaQris1.SenderEmail != "bca@bca.co.id" {
		t.Fatalf("expected normalized sender bca@bca.co.id, got %q", bcaQris1.SenderEmail)
	}
	if bcaQris1.BodyCue != "transaction type: qris payment" {
		t.Fatalf("expected Pass A multi-line cue 'transaction type: qris payment', got %q", bcaQris1.BodyCue)
	}
	if bcaQris1.SignatureKey != bcaQris2.SignatureKey {
		t.Fatalf("expected identical QRIS signatures, got %q vs %q", bcaQris1.SignatureKey, bcaQris2.SignatureKey)
	}

	bcaTransfer := DeriveEmailTemplateSignature(
		"bca@bca.co.id",
		"Internet Transaction Journal",
		"Hi MUHAMMAD FARIZ,\nYou just made a transaction through myBCA.\nHere are the details of your transaction :\nStatus\n:\nSuccessful\nTransaction Date\n:\n24 Nov 2026 11:34:40\nTransfer Type\n:\nTransfer to BCA Account",
	)
	if bcaTransfer.BodyCue != "transfer type: transfer to bca account" {
		t.Fatalf("expected BCA transfer cue 'transfer type: transfer to bca account', got %q", bcaTransfer.BodyCue)
	}
	if bcaTransfer.SignatureKey == bcaQris1.SignatureKey {
		t.Fatalf("expected BCA transfer signature to differ from BCA QRIS")
	}

	grabTips1 := DeriveEmailTemplateSignature(
		"no-reply@grab.com",
		"Your Grab E-Receipt",
		"Grab Tips E-Receipt\nGrabFood\nTerima kasih! Tip darimu sudah disalurkan ke pengemudimu.\n23 Nov 26 20:14 +0700\nTotal\nRP 5000",
	)
	grabTips2 := DeriveEmailTemplateSignature(
		"no-reply@grab.com",
		"Your Grab E-Receipt",
		"Grab Tips E-Receipt\nBike Standard\nThanks! Your tip goes a long way for your driver.\n22 Nov 26 11:02 +0700",
	)
	grabFood := DeriveEmailTemplateSignature(
		"no-reply@grab.com",
		"Your Grab E-Receipt",
		"Selamat menikmati makanan Anda!\nTOTAL\nRp 38355\n23 Nov 26 20:11 +0700",
	)
	grabBike := DeriveEmailTemplateSignature(
		"no-reply@grab.com",
		"Your Grab E-Receipt",
		"Grab E-Receipt\nBike Standard\nHope you enjoyed your ride!\nPicked up on 22 November 2026",
	)
	if grabTips1.BodyCue != "grab tips e-receipt" || grabTips1.SignatureKey != grabTips2.SignatureKey {
		t.Fatalf("expected matching grab tips cue/key, got cue=%q key1=%q key2=%q", grabTips1.BodyCue, grabTips1.SignatureKey, grabTips2.SignatureKey)
	}
	if grabFood.BodyCue != "selamat menikmati makanan anda!" {
		t.Fatalf("expected grab food body cue, got %q", grabFood.BodyCue)
	}
	if grabBike.BodyCue != "grab e-receipt" {
		t.Fatalf("expected grab bike body cue 'grab e-receipt', got %q", grabBike.BodyCue)
	}
	if grabTips1.SignatureKey == grabFood.SignatureKey || grabTips1.SignatureKey == grabBike.SignatureKey || grabFood.SignatureKey == grabBike.SignatureKey {
		t.Fatalf("expected distinct Grab tip, food, and bike signatures")
	}

	voucher1 := DeriveEmailTemplateSignature(
		"no-reply@grab.com",
		"Ride Cover Plus: Sorry for the long wait. Here's your Rp11.000 voucher.",
		"Ride Cover\nRide Cover Plus: Sorry for the long wait. Here's your Rp11.000 voucher.\nHi Nagib Fariz,",
	)
	voucher2 := DeriveEmailTemplateSignature(
		"no-reply@grab.com",
		"Ride Cover Plus: Sorry for the long wait. Here's your Rp25.000 voucher.",
		"Ride Cover\nRide Cover Plus: Sorry for the long wait. Here's your Rp25.000 voucher.\nHi Another User,",
	)
	if voucher1.SignatureKey != voucher2.SignatureKey {
		t.Fatalf("expected Grab Ride Cover vouchers to share signature key (%q vs %q, subj %q vs %q)", voucher1.SignatureKey, voucher2.SignatureKey, voucher1.SubjectPattern, voucher2.SubjectPattern)
	}

	jeniusCC1 := DeriveEmailTemplateSignature(
		"jenius_noreply@smbci.com",
		"Credit Card Transaction Notification",
		"Hi Muhammad Fariz,\nThank you for using Jenius Credit Card.\nCard number: 437896******6039",
	)
	jeniusCancel1 := DeriveEmailTemplateSignature(
		"jenius_noreply@smbci.com",
		"Your Jenius 437896******6039 transaction has been cancelled",
		"Hi Muhammad Fariz,\nThank you for using the Jenius Credit Card.",
	)
	jeniusCancel2 := DeriveEmailTemplateSignature(
		"jenius_noreply@smbci.com",
		"Your Jenius 437896******1111 transaction has been cancelled",
		"Hi Muhammad Fariz,\nThank you for using the Jenius Credit Card.",
	)
	jeniusTransfer := DeriveEmailTemplateSignature(
		"jenius_noreply@smbci.com",
		"You've transferred money from your Jenius account",
		"Hi Muhammad,\nYou have just transferred IDR100,000 to ANKIE ERTMA RAHFREEL.",
	)
	if jeniusCancel1.SignatureKey != jeniusCancel2.SignatureKey {
		t.Fatalf("expected masked Jenius card cancellations to match: %q vs %q", jeniusCancel1.SubjectPattern, jeniusCancel2.SubjectPattern)
	}
	if jeniusTransfer.SubjectPattern != "you've transferred money from your jenius account" {
		t.Fatalf("expected static amount-less Jenius transfer subject preserved, got %q", jeniusTransfer.SubjectPattern)
	}
	if jeniusCC1.SignatureKey == jeniusCancel1.SignatureKey || jeniusCC1.SignatureKey == jeniusTransfer.SignatureKey {
		t.Fatalf("expected distinct Jenius charge, cancel, and transfer signatures")
	}

	shopeeOrder1 := DeriveEmailTemplateSignature(
		"info@mail.shopee.co.id",
		"Pesanan\u00a0#26112206P1FVP9 Telah Dikirim",
		"Pesanan #26112206P1FVP9 Telah Dikirim\nHai keynesfariz,",
	)
	shopeeOrder2 := DeriveEmailTemplateSignature(
		"info@mail.shopee.co.id",
		"Pesanan #261116EV2QVT1B Telah Dikirim",
		"Pesanan #261116EV2QVT1B Telah Dikirim\nHai keynesfariz,",
	)
	shopeeSec := DeriveEmailTemplateSignature(
		"info@mail.shopee.co.id",
		"Kami mendeteksi Aktivitas Penautan Akun baru di Akun Shopee Anda",
		"Hai keynesfariz,\nSebuah akun Migoo baru-baru ini ditautkan ke Akun Shopee Anda.",
	)
	if shopeeOrder1.SignatureKey != shopeeOrder2.SignatureKey {
		t.Fatalf("expected Shopee orders to match across IDs and NBSP: %q vs %q", shopeeOrder1.SubjectPattern, shopeeOrder2.SubjectPattern)
	}
	if shopeeOrder1.SignatureKey == shopeeSec.SignatureKey {
		t.Fatalf("expected Shopee order signature to differ from security alert")
	}

	zelle1 := DeriveEmailTemplateSignature("alerts@usbank.example", "You sent $42.00 via Zelle to Alex", "")
	zelle2 := DeriveEmailTemplateSignature("alerts@usbank.example", "You sent $15.50 via Zelle to Jordan", "")
	zelleSec := DeriveEmailTemplateSignature("alerts@usbank.example", "Security alert: new login", "")
	if zelle1.SubjectPattern != "you sent {amount} via zelle to {counterparty}" {
		t.Fatalf("unexpected Zelle subject pattern: %q", zelle1.SubjectPattern)
	}
	if zelle1.SignatureKey != zelle2.SignatureKey || zelle1.SignatureKey == zelleSec.SignatureKey {
		t.Fatalf("expected Zelle transfer signatures to match each other and differ from security alert")
	}

	payPreAmount := DeriveEmailTemplateSignature("alerts@usbank.example", "Payment to Starbucks for $12.50", "")
	if payPreAmount.SubjectPattern != "payment to {counterparty} for {amount}" {
		t.Fatalf("expected 'payment to {counterparty} for {amount}' without consuming {amount}, got %q", payPreAmount.SubjectPattern)
	}

	txSucceeded := DeriveEmailTemplateSignature("alerts@usbank.example", "Transfer of $50 to Alice succeeded", "")
	txFailed := DeriveEmailTemplateSignature("alerts@usbank.example", "Transfer of $50 to Alice failed", "")
	if txSucceeded.SignatureKey == txFailed.SignatureKey {
		t.Fatalf("expected succeeded (%q) and failed (%q) outcome signatures to stay distinct", txSucceeded.SubjectPattern, txFailed.SubjectPattern)
	}

	singleWordAndGreeting := DeriveEmailTemplateSignature(
		"no-reply@grab.com",
		"Weekly digest",
		"Grab\nHai keynesfariz,\nWe processed your request today.",
	)
	if singleWordAndGreeting.BodyCue != "" {
		t.Fatalf("expected empty body cue when header block only has single-word brand and greeting, got %q", singleWordAndGreeting.BodyCue)
	}
}
