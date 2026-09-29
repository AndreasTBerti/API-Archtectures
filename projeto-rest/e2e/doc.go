// Package e2e tem os testes de ponta a ponta do projeto-rest. Eles sobem o
// serviço SOAP real (../projeto-soap, Python) e o binário compilado do REST,
// cada um com um banco temporário, e testam as jornadas pela rede (HTTP e
// WebSocket), como um cliente de verdade.
//
// Só rodam com a tag e2e:
//
//	go test -tags e2e ./e2e
package e2e
