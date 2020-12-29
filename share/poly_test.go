package share

import (
	"testing"

	"github.com/DOSNetwork/core/suites"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//var suite = suites.MustFind("Ed25519")
var suite = suites.MustFind("bn256")

func TestSecretRecovery(test *testing.T) {
	n := 10
	t := n/2 + 1
	poly := NewPriPoly(suite, t, nil, suite.RandomStream())
	shares := poly.Shares(n)

	recovered, err := RecoverSecret(suite, shares, t, n)
	if err != nil {
		test.Fatal(err)
	}

	if !recovered.Equal(poly.Secret()) {
		test.Fatal("recovered secret does not match initial value")
	}
}

func TestSecretRecoveryDelete(test *testing.T) {
	n := 10
	t := n/2 + 1
	poly := NewPriPoly(suite, t, nil, suite.RandomStream())
	shares := poly.Shares(n)

	// Corrupt a few shares
	shares[2] = nil
	shares[5] = nil
	shares[7] = nil
	shares[8] = nil

	recovered, err := RecoverSecret(suite, shares, t, n)
	if err != nil {
		test.Fatal(err)
	}

	if !recovered.Equal(poly.Secret()) {
		test.Fatal("recovered secret does not match initial value")
	}
}

func TestSecretRecoveryDeleteFail(test *testing.T) {
	n := 10
	t := n/2 + 1

	poly := NewPriPoly(suite, t, nil, suite.RandomStream())
	shares := poly.Shares(n)

	// Corrupt one more share than acceptable
	shares[1] = nil
	shares[2] = nil
	shares[5] = nil
	shares[7] = nil
	shares[8] = nil

	_, err := RecoverSecret(suite, shares, t, n)
	if err == nil {
		test.Fatal("recovered secret unexpectably")
	}
}

func TestSecretPolyEqual(test *testing.T) {
	n := 10
	t := n/2 + 1

	p1 := NewPriPoly(suite, t, nil, suite.RandomStream())
	p2 := NewPriPoly(suite, t, nil, suite.RandomStream())
	p3 := NewPriPoly(suite, t, nil, suite.RandomStream())

	p12, _ := p1.Add(p2)
	p13, _ := p1.Add(p3)

	p123, _ := p12.Add(p3)
	p132, _ := p13.Add(p2)

	if !p123.Equal(p132) {
		test.Fatal("private polynomials not equal")
	}
}

func TestPublicCheck(test *testing.T) {
	n := 10
	t := n/2 + 1

	priPoly := NewPriPoly(suite, t, nil, suite.RandomStream())
	priShares := priPoly.Shares(n)
	pubPoly := priPoly.Commit(nil)

	for i, share := range priShares {
		if !pubPoly.Check(share) {
			test.Fatalf("private share %v not valid with respect to the public commitment polynomial", i)
		}
	}
}

func TestPublicRecovery(test *testing.T) {
	n := 10
	t := n/2 + 1

	priPoly := NewPriPoly(suite, t, nil, suite.RandomStream())
	pubPoly := priPoly.Commit(nil)
	pubShares := pubPoly.Shares(n)

	recovered, err := RecoverCommit(suite, pubShares, t, n)
	if err != nil {
		test.Fatal(err)
	}

	if !recovered.Equal(pubPoly.Commit()) {
		test.Fatal("recovered commit does not match initial value")
	}
}

func TestPublicRecoveryDelete(test *testing.T) {
	n := 10
	t := n/2 + 1

	priPoly := NewPriPoly(suite, t, nil, suite.RandomStream())
	pubPoly := priPoly.Commit(nil)
	shares := pubPoly.Shares(n)

	// Corrupt a few shares
	shares[2] = nil
	shares[5] = nil
	shares[7] = nil
	shares[8] = nil

	recovered, err := RecoverCommit(suite, shares, t, n)
	if err != nil {
		test.Fatal(err)
	}

	if !recovered.Equal(pubPoly.Commit()) {
		test.Fatal("recovered commit does not match initial value")
	}
}

func TestPublicRecoveryDeleteFail(test *testing.T) {
	n := 10
	t := n/2 + 1

	priPoly := NewPriPoly(suite, t, nil, suite.RandomStream())
	pubPoly := priPoly.Commit(nil)
	shares := pubPoly.Shares(n)

	// Corrupt one more share than acceptable
	shares[1] = nil
	shares[2] = nil
	shares[5] = nil
	shares[7] = nil
	shares[8] = nil

	_, err := RecoverCommit(suite, shares, t, n)
	if err == nil {
		test.Fatal("recovered commit unexpectably")
	}
}

func TestPrivateAdd(test *testing.T) {
	n := 10
	t := n/2 + 1

	p := NewPriPoly(suite, t, nil, suite.RandomStream())
	q := NewPriPoly(suite, t, nil, suite.RandomStream())

	r, err := p.Add(q)
	if err != nil {
		test.Fatal(err)
	}

	ps := p.Secret()
	qs := q.Secret()
	rs := suite.Scalar().Add(ps, qs)

	if !rs.Equal(r.Secret()) {
		test.Fatal("addition of secret sharing polynomials failed")
	}
}

func TestPublicAdd(test *testing.T) {
	n := 10
	t := n/2 + 1

	G := suite.Point().Pick(suite.RandomStream())
	H := suite.Point().Pick(suite.RandomStream())

	p := NewPriPoly(suite, t, nil, suite.RandomStream())
	q := NewPriPoly(suite, t, nil, suite.RandomStream())

	P := p.Commit(G)
	Q := q.Commit(H)

	R, err := P.Add(Q)
	if err != nil {
		test.Fatal(err)
	}

	shares := R.Shares(n)
	recovered, err := RecoverCommit(suite, shares, t, n)
	if err != nil {
		test.Fatal(err)
	}

	x := P.Commit()
	y := Q.Commit()
	z := suite.Point().Add(x, y)

	if !recovered.Equal(z) {
		test.Fatal("addition of public commitment polynomials failed")
	}
}

func TestPublicPolyEqual(test *testing.T) {
	n := 10
	t := n/2 + 1

	G := suite.Point().Pick(suite.RandomStream())

	p1 := NewPriPoly(suite, t, nil, suite.RandomStream())
	p2 := NewPriPoly(suite, t, nil, suite.RandomStream())
	p3 := NewPriPoly(suite, t, nil, suite.RandomStream())

	P1 := p1.Commit(G)
	P2 := p2.Commit(G)
	P3 := p3.Commit(G)

	P12, _ := P1.Add(P2)
	P13, _ := P1.Add(P3)

	P123, _ := P12.Add(P3)
	P132, _ := P13.Add(P2)

	if !P123.Equal(P132) {
		test.Fatal("public polynomials not equal")
	}
}

func TestPriPolyMul(test *testing.T) {
	n := 10
	t := n/2 + 1
	a := NewPriPoly(suite, t, nil, suite.RandomStream())
	b := NewPriPoly(suite, t, nil, suite.RandomStream())

	c := a.Mul(b)
	assert.Equal(test, len(a.coeffs)+len(b.coeffs)-1, len(c.coeffs))
	nul := suite.Scalar().Zero()
	for _, coeff := range c.coeffs {
		assert.NotEqual(test, nul.String(), coeff.String())
	}

	a0 := a.coeffs[0]
	b0 := b.coeffs[0]
	mul := suite.Scalar().Mul(b0, a0)
	c0 := c.coeffs[0]
	assert.Equal(test, c0.String(), mul.String())

	at := a.coeffs[len(a.coeffs)-1]
	bt := b.coeffs[len(b.coeffs)-1]
	mul = suite.Scalar().Mul(at, bt)
	ct := c.coeffs[len(c.coeffs)-1]
	assert.Equal(test, ct.String(), mul.String())
}

func TestRecoverPriPoly(test *testing.T) {
	n := 10
	t := n/2 + 1
	a := NewPriPoly(suite, t, nil, suite.RandomStream())

	shares := a.Shares(n)
	reverses := make([]*PriShare, len(shares))
	l := len(shares) - 1
	for i := range shares {
		reverses[l-i] = shares[i]
	}
	recovered, err := RecoverPriPoly(suite, shares, t, n)
	assert.Nil(test, err)

	reverseRecovered, err := RecoverPriPoly(suite, reverses, t, n)
	assert.Nil(test, err)

	for i := 0; i < t; i++ {
		assert.Equal(test, recovered.Eval(i).V.String(), a.Eval(i).V.String())
		assert.Equal(test, reverseRecovered.Eval(i).V.String(), a.Eval(i).V.String())
	}
}

func TestPriPolyCoefficients(test *testing.T) {
	n := 10
	t := n/2 + 1
	a := NewPriPoly(suite, t, nil, suite.RandomStream())

	coeffs := a.Coefficients()
	require.Len(test, coeffs, t)

	b := CoefficientsToPriPoly(suite, coeffs)
	require.Equal(test, a.coeffs, b.coeffs)

}
