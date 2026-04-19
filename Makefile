build:
	go build ./cmd/a4grid
compose.m5:
	./a4grid -input input/set1 -output output/pack-m5.pdf -margin 0
compose.m0:
	./a4grid -input input/set1 -output output/pack-m0.pdf -margin 0
compose.from.pdf.m5:
	./a4grid -input ./input/pdfs/postcards.pdf -output output/postcards.pdf -margin 5