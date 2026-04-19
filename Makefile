build: build.a4grid build.ticketpack
build.a4grid:
	go build ./cmd/a4grid
build.ticketpack:
	go build ./cmd/ticketpack
compose.m5:
	./a4grid -input input/set1 -output output/pack-m5.pdf -margin 0
compose.m0:
	./a4grid -input input/set1 -output output/pack-m0.pdf -margin 0
compose.from.pdf.m5:
	./a4grid -input ./input/pdfs/postcards.pdf -output output/postcards.pdf -margin 5
compose.tickets:
	./ticketpack -input input/tickets -output output/boarding-a4.pdf