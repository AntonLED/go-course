package mtls

// Identity — кто на другой стороне mTLS-соединения (из проверенного
// клиентского сертификата).
type Identity struct {
	CommonName string   // Subject.CommonName
	DNSNames   []string // SAN DNS
	URIs       []string // SAN URI, например SPIFFE ID "spiffe://example.org/ns/prod/sa/orders"
}

// Names возвращает все имена идентичности: CN, DNS SAN и URI SAN.
func (id Identity) Names() []string {
	names := make([]string, 0, 1+len(id.DNSNames)+len(id.URIs))
	if id.CommonName != "" {
		names = append(names, id.CommonName)
	}
	names = append(names, id.DNSNames...)
	return append(names, id.URIs...)
}
