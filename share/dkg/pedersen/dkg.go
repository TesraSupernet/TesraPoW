// Package dkg implements the protocol described in "A threshold cryptosystem without a trusted party"
// by Torben Pryds Pedersen. https://dl.acm.org/citation.cfm?id=1754929.
package dkg

import (
	"errors"

	"github.com/DOSNetwork/core/share"
	vss "github.com/DOSNetwork/core/share/vss/pedersen"
	"github.com/DOSNetwork/core/suites"
	"github.com/dedis/kyber"
)

// Suite wraps the functionalities needed by the dkg package
type Suite suites.Suite

// DistKeyShare holds the share of a distributed key for a participant.
type DistKeyShare struct {
	// Coefficients of the public polynomial holding the public key.
	Commits []kyber.Point
	// Share of the distributed secret which is private information.
	Share *share.PriShare
	// Coefficients of the private polynomial generated by the node holding the
	// share. The final distributed polynomial is the sum of all these
	// individual polynomials, but it is never computed.
	PrivatePoly []kyber.Scalar
}

// Public returns the public key associated with the distributed private key.
func (d *DistKeyShare) Public() kyber.Point {
	return d.Commits[0]
}

// PriShare implements the dss.DistKeyShare interface so either pedersen or
// rabin dkg can be used with dss.
func (d *DistKeyShare) PriShare() *share.PriShare {
	return d.Share
}

// Commitments implements the dss.DistKeyShare interface so either pedersen or
// rabin dkg can be used with dss.
func (d *DistKeyShare) Commitments() []kyber.Point {
	return d.Commits
}

// Justification holds the Justification from a Dealer as well as the index of
// the Dealer in question.
type Justification struct {
	// Index of the Dealer who answered with this Justification
	Index uint32
	// Justification issued from the Dealer
	Justification *vss.Justification
}

// DistKeyGenerator is the struct that runs the DKG protocol.
type DistKeyGenerator struct {
	suite Suite

	index uint32
	long  kyber.Scalar
	pub   kyber.Point

	participants []kyber.Point

	t int

	dealer    *vss.Dealer
	verifiers map[uint32]*vss.Verifier
}

// initDistKeyGenerator returns a dist key generator with the given secret as
// the first coefficient of the polynomial of this dealer.
func initDistKeyGenerator(suite Suite, longterm kyber.Scalar, participants []kyber.Point, t int, secret kyber.Scalar) (*DistKeyGenerator, error) {
	pub := suite.Point().Mul(longterm, nil)
	// find our index
	var found bool
	var index uint32
	for i, p := range participants {
		if p.Equal(pub) {
			found = true
			index = uint32(i)
			break
		}
	}
	if !found {
		return nil, errors.New("dkg: own public key not found in list of participants")
	}
	var err error
	// generate our dealer / deal
	ownSec := secret
	dealer, err := vss.NewDealer(suite, longterm, ownSec, participants, t)
	if err != nil {
		return nil, err
	}

	return &DistKeyGenerator{
		dealer:       dealer,
		verifiers:    make(map[uint32]*vss.Verifier),
		t:            t,
		suite:        suite,
		long:         longterm,
		pub:          pub,
		participants: participants,
		index:        index,
	}, nil
}

// NewDistKeyGenerator returns a DistKeyGenerator out of the suite, the longterm
// secret key, the list of participants, the threshold t parameter and a given
// secret. It returns an error if the secret key's commitment can't be found in
// the list of participants.
func NewDistKeyGenerator(suite Suite, longterm kyber.Scalar, participants []kyber.Point, t int) (*DistKeyGenerator, error) {
	ownSecret := suite.Scalar().Pick(suite.RandomStream())
	return initDistKeyGenerator(suite, longterm, participants, t, ownSecret)
}

// NewDistKeyGeneratorWithoutSecret simply returns a DistKeyGenerator with an
// nil secret.  It is used to renew the private shares without affecting the
// secret.
func NewDistKeyGeneratorWithoutSecret(suite Suite, longterm kyber.Scalar, participants []kyber.Point, t int) (*DistKeyGenerator, error) {
	ownSecret := suite.Scalar().Zero()
	return initDistKeyGenerator(suite, longterm, participants, t, ownSecret)
}

// Deals returns all the deals that must be broadcasted to all
// participants. The deal corresponding to this DKG is already added
// to this DKG and is omitted from the returned map. To know which
// participant a deal belongs to, loop over the keys as indices in the
// list of participants:
//
//   for i,dd := range distDeals {
//      sendTo(participants[i],dd)
//   }
//
// If this method cannot process its own Deal, that indicates a
// sever problem with the configuration or implementation and
// results in a panic.
func (d *DistKeyGenerator) Deals() (map[int]*Deal, error) {
	deals, err := d.dealer.EncryptedDeals()
	if err != nil {
		return nil, err
	}
	dd := make(map[int]*Deal)
	for i := range d.participants {
		distd := &Deal{
			Index: d.index,
			Deal:  deals[i],
		}
		if i == int(d.index) {
			if _, ok := d.verifiers[d.index]; ok {
				// already processed our own deal
				continue
			}
			if resp, err := d.ProcessDeal(distd); err != nil {
				panic("dkg: cannot process own deal: " + err.Error())
			} else if resp.Response.Status != vss.StatusApproval {
				panic("dkg: own deal gave a complaint")
			}
			continue
		}
		dd[i] = distd
	}
	return dd, nil
}

// ProcessDeal takes a Deal created by Deals() and stores and verifies it. It
// returns a Response to broadcast to every other participant. It returns an
// error in case the deal has already been stored, or if the deal is incorrect
// (see vss.Verifier.ProcessEncryptedDeal).
func (d *DistKeyGenerator) ProcessDeal(dd *Deal) (*Response, error) {
	// public key of the dealer
	pub, ok := findPub(d.participants, dd.Index)
	if !ok {
		return nil, errors.New("dkg: dist deal out of bounds index")
	}

	if _, ok := d.verifiers[dd.Index]; ok {
		return nil, errors.New("dkg: already received dist deal from same index")
	}

	// verifier receiving the dealer's deal
	ver, err := vss.NewVerifier(d.suite, d.long, pub, d.participants)
	if err != nil {
		return nil, err
	}

	d.verifiers[dd.Index] = ver
	resp, err := ver.ProcessEncryptedDeal(dd.Deal)
	if err != nil {
		return nil, err
	}

	// Set StatusApproval for the verifier that represents the participant
	// that distibuted the Deal
	d.verifiers[dd.Index].UnsafeSetResponseDKG(dd.Index, vss.StatusApproval)

	return &Response{
		Index:    dd.Index,
		Response: resp,
	}, nil
}

// ProcessResponse takes a response from every other peer.  If the response
// designates the deal of another participant than this dkg, this dkg stores it
// and returns nil with a possible error regarding the validity of the response.
// If the response designates a deal this dkg has issued, then the dkg will process
// the response, and returns a justification.
func (d *DistKeyGenerator) ProcessResponse(resp *Response) (*Justification, error) {
	v, ok := d.verifiers[resp.Index]
	if !ok {
		return nil, errors.New("dkg: complaint received but no deal for it")
	}

	if err := v.ProcessResponse(resp.Response); err != nil {
		return nil, err
	}

	if resp.Index != uint32(d.index) {
		return nil, nil
	}

	j, err := d.dealer.ProcessResponse(resp.Response)
	if err != nil {
		return nil, err
	}
	if j == nil {
		return nil, nil
	}
	// a justification for our own deal, are we cheating !?
	if err := v.ProcessJustification(j); err != nil {
		return nil, err
	}

	return &Justification{
		Index:         d.index,
		Justification: j,
	}, nil
}

// ProcessJustification takes a justification and validates it. It returns an
// error in case the justification is wrong.
func (d *DistKeyGenerator) ProcessJustification(j *Justification) error {
	v, ok := d.verifiers[j.Index]
	if !ok {
		return errors.New("dkg: Justification received but no deal for it")
	}
	return v.ProcessJustification(j.Justification)
}

// SetTimeout triggers the timeout on all verifiers, and thus makes sure
// all verifiers have either responded, or have a StatusComplaint response.
func (d *DistKeyGenerator) SetTimeout() {
	for _, v := range d.verifiers {
		v.SetTimeout()
	}
}

// Certified returns true if at least t deals are certified (see
// vss.Verifier.DealCertified()). If the distribution is certified, the protocol
// can continue using d.SecretCommits().
func (d *DistKeyGenerator) Certified() bool {
	return len(d.QUAL()) >= len(d.participants)
}

// QUAL returns the index in the list of participants that forms the QUALIFIED
// set as described in the "New-DKG" protocol by Rabin. Basically, it consists
// of all participants that are not disqualified after having  exchanged all
// deals, responses and justification. This is the set that is used to extract
// the distributed public key with SecretCommits() and ProcessSecretCommits().
func (d *DistKeyGenerator) QUAL() []int {
	var good []int
	d.qualIter(func(i uint32, v *vss.Verifier) bool {
		good = append(good, int(i))
		return true
	})
	return good
}

func (d *DistKeyGenerator) isInQUAL(idx uint32) bool {
	var found bool
	d.qualIter(func(i uint32, v *vss.Verifier) bool {
		if i == idx {
			found = true
			return false
		}
		return true
	})
	return found
}

func (d *DistKeyGenerator) qualIter(fn func(idx uint32, v *vss.Verifier) bool) {
	for i, v := range d.verifiers {
		if v.DealCertified() {
			if !fn(i, v) {
				break
			}
		}
	}
}

// DistKeyShare generates the distributed key relative to this receiver.
// It throws an error if something is wrong such as not enough deals received.
// The shared secret can be computed when all deals have been sent and
// basically consists of a public point and a share. The public point is the sum
// of all aggregated individual public commits of each individual secrets.
// the share is evaluated from the global Private Polynomial, basically SUM of
// fj(i) for a receiver i.
func (d *DistKeyGenerator) DistKeyShare() (*DistKeyShare, error) {
	if !d.Certified() {
		return nil, errors.New("dkg: distributed key not certified")
	}

	sh := d.suite.Scalar().Zero()
	var pub *share.PubPoly
	var err error

	d.qualIter(func(i uint32, v *vss.Verifier) bool {
		// share of dist. secret = sum of all share received.
		deal := v.Deal()
		s := deal.SecShare.V
		sh = sh.Add(sh, s)
		// Dist. public key = sum of all revealed commitments
		poly := share.NewPubPoly(d.suite, d.suite.Point().Base(), deal.Commitments)
		if pub == nil {
			// first polynomial we see (instead of generating n empty commits)
			pub = poly
			return true
		}
		pub, err = pub.Add(poly)
		return err == nil
	})

	if err != nil {
		return nil, err
	}
	_, commits := pub.Info()

	return &DistKeyShare{
		Commits: commits,
		Share: &share.PriShare{
			I: int(d.index),
			V: sh,
		},
		PrivatePoly: d.dealer.PrivatePoly().Coefficients(),
	}, nil
}

//Renew adds the new distributed key share g (with secret 0) to the distributed key share d.
func (d *DistKeyShare) Renew(suite Suite, g *DistKeyShare) (*DistKeyShare, error) {
	//Check G(0) = 0*G.
	if !g.Public().Equal(suite.Point().Base().Mul(suite.Scalar().Zero(), nil)) {
		return nil, errors.New("wrong renewal function")
	}

	//Check whether they have the same index
	if d.Share.I != g.Share.I {
		return nil, errors.New("not the same party")
	}

	newShare := suite.Scalar().Add(d.Share.V, g.Share.V)
	newCommits := make([]kyber.Point, len(d.Commits))
	for i := range newCommits {
		newCommits[i] = suite.Point().Add(d.Commits[i], g.Commits[i])
	}
	return &DistKeyShare{
		Commits: newCommits,
		Share: &share.PriShare{
			I: d.Share.I,
			V: newShare,
		},
	}, nil
}

func findPub(list []kyber.Point, i uint32) (kyber.Point, bool) {
	if i >= uint32(len(list)) {
		return nil, false
	}
	return list[i], true
}
