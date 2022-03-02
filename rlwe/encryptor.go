package rlwe

import (
	"github.com/tuneinsight/lattigo/v3/ring"
	"github.com/tuneinsight/lattigo/v3/utils"
	"math/big"
)

// Encryptor a generic RLWE encryption interface.
type Encryptor interface {
	Encrypt(pt *Plaintext, ct *Ciphertext)
	EncryptFromCRP(pt *Plaintext, crp *ring.Poly, ct *Ciphertext)
	EncryptRGSW(pt *Plaintext, ct *RGSWCiphertext)
	ShallowCopy() Encryptor
	WithKey(key interface{}) Encryptor
}

type encryptor struct {
	*encryptorBase
	*encryptorSamplers
	*encryptorBuffers
	basisextender *ring.BasisExtender
}

type pkEncryptor struct {
	encryptor
	pk *PublicKey
}

type skEncryptor struct {
	encryptor
	sk *SecretKey
}

// NewEncryptor creates a new Encryptor
// Accepts either a secret-key or a public-key.
func NewEncryptor(params Parameters, key interface{}) Encryptor {
	enc := newEncryptor(params)
	return enc.setKey(key)
}

func newEncryptor(params Parameters) encryptor {

	var bc *ring.BasisExtender
	if params.PCount() != 0 {
		bc = ring.NewBasisExtender(params.RingQ(), params.RingP())
	}

	return encryptor{
		encryptorBase:     newEncryptorBase(params),
		encryptorSamplers: newEncryptorSamplers(params),
		encryptorBuffers:  newEncryptorBuffers(params),
		basisextender:     bc,
	}
}

// encryptorBase is a struct used to encrypt Plaintexts. It stores the public-key and/or secret-key.
type encryptorBase struct {
	params Parameters
}

func newEncryptorBase(params Parameters) *encryptorBase {
	return &encryptorBase{params}
}

type encryptorSamplers struct {
	gaussianSampler *ring.GaussianSampler
	ternarySampler  *ring.TernarySampler
	uniformSamplerQ *ring.UniformSampler
	uniformSamplerP *ring.UniformSampler
}

func newEncryptorSamplers(params Parameters) *encryptorSamplers {
	prng, err := utils.NewPRNG()
	if err != nil {
		panic(err)
	}

	var uniformSamplerP *ring.UniformSampler
	if params.PCount() != 0 {
		uniformSamplerP = ring.NewUniformSampler(prng, params.RingP())
	}

	return &encryptorSamplers{
		gaussianSampler: ring.NewGaussianSampler(prng, params.RingQ(), params.Sigma(), int(6*params.Sigma())),
		ternarySampler:  ring.NewTernarySamplerWithHammingWeight(prng, params.ringQ, params.h, false),
		uniformSamplerQ: ring.NewUniformSampler(prng, params.RingQ()),
		uniformSamplerP: uniformSamplerP,
	}
}

type encryptorBuffers struct {
	poolQ [2]*ring.Poly
	poolP [3]*ring.Poly
}

func newEncryptorBuffers(params Parameters) *encryptorBuffers {

	ringQ := params.RingQ()
	ringP := params.RingP()

	var poolP [3]*ring.Poly
	if params.PCount() != 0 {
		poolP = [3]*ring.Poly{ringP.NewPoly(), ringP.NewPoly(), ringP.NewPoly()}
	}

	return &encryptorBuffers{
		poolQ: [2]*ring.Poly{ringQ.NewPoly(), ringQ.NewPoly()},
		poolP: poolP,
	}
}

// Encrypt encrypts the input plaintext using the stored public-key and writes the result on ct.
// The encryption procedure first samples an new encryption of zero under the public-key and
// then adds the plaintext.
// The encryption procedures depends on the parameters. If the auxiliary modulus P is defined,
// then the encryption of zero is sampled in QP before being rescaled by P; otherwise, it is directly
// sampled in Q.
func (enc *pkEncryptor) Encrypt(pt *Plaintext, ct *Ciphertext) {
	enc.uniformSamplerQ.ReadLvl(utils.MinInt(pt.Level(), ct.Level()), ct.Value[1])

	if enc.basisextender != nil {
		enc.encrypt(pt, ct)
	} else {
		enc.encryptNoP(pt, ct)
	}
}

// EncryptFromCRP is not defined when using a public-key. This method will panic.
func (enc *pkEncryptor) EncryptFromCRP(pt *Plaintext, crp *ring.Poly, ct *Ciphertext) {
	panic("Cannot encrypt with CRP using a public-key")
}

// Encrypt encrypts the input plaintext and write the result on ct.
func (enc *skEncryptor) Encrypt(pt *Plaintext, ct *Ciphertext) {

	enc.uniformSamplerQ.ReadLvl(utils.MinInt(pt.Level(), ct.Level()), ct.Value[1])

	enc.encrypt(pt, ct)
}

// EncryptFromCRP encrypts the input plaintext and writes the result on ct.
// The encryption algorithm depends on the implementor.
func (enc *skEncryptor) EncryptFromCRP(pt *Plaintext, crp *ring.Poly, ct *Ciphertext) {
	ring.CopyValues(crp, ct.Value[1])

	enc.encrypt(pt, ct)
}

// ShallowCopy creates a shallow copy of this pkEncryptor in which all the read-only data-structures are
// shared with the receiver and the temporary buffers are reallocated. The receiver and the returned
// Encryptors can be used concurrently.
func (enc *pkEncryptor) ShallowCopy() Encryptor {
	return &pkEncryptor{*enc.encryptor.ShallowCopy(), enc.pk}
}

// ShallowCopy creates a shallow copy of this skEncryptor in which all the read-only data-structures are
// shared with the receiver and the temporary buffers are reallocated. The receiver and the returned
// Encryptors can be used concurrently.
func (enc *skEncryptor) ShallowCopy() Encryptor {
	return &skEncryptor{*enc.encryptor.ShallowCopy(), enc.sk}
}

// ShallowCopy creates a shallow copy of this encryptor in which all the read-only data-structures are
// shared with the receiver and the temporary buffers are reallocated. The receiver and the returned
// Encryptors can be used concurrently.
func (enc *encryptor) ShallowCopy() *encryptor {

	var bc *ring.BasisExtender
	if enc.params.PCount() != 0 {
		bc = enc.basisextender.ShallowCopy()
	}

	return &encryptor{
		encryptorBase:     enc.encryptorBase,
		encryptorSamplers: newEncryptorSamplers(enc.params),
		encryptorBuffers:  newEncryptorBuffers(enc.params),
		basisextender:     bc,
	}
}

// WithKey creates a shallow copy of this encryptor with a new key in which all the read-only data-structures are
// shared with the receiver and the temporary buffers are reallocated. The receiver and the returned
// Encryptors can be used concurrently.
func (enc *encryptor) WithKey(key interface{}) Encryptor {
	return enc.ShallowCopy().setKey(key)
}

func (enc *pkEncryptor) encrypt(plaintext *Plaintext, ciphertext *Ciphertext) {
	ringQ := enc.params.RingQ()
	ringQP := enc.params.RingQP()

	levelQ := utils.MinInt(plaintext.Level(), ciphertext.Level())
	levelP := 0

	poolQ0 := enc.poolQ[0]
	poolP0 := enc.poolP[0]
	poolP1 := enc.poolP[1]
	poolP2 := enc.poolP[2]

	// We sample a R-WLE instance (encryption of zero) over the extended ring (ciphertext ring + special prime)

	ciphertextNTT := ciphertext.Value[0].IsNTT

	u := PolyQP{Q: poolQ0, P: poolP2}

	enc.ternarySampler.ReadLvl(levelQ, u.Q)
	ringQP.ExtendBasisSmallNormAndCenter(u.Q, levelP, nil, u.P)

	// (#Q + #P) NTT
	ringQP.NTTLvl(levelQ, levelP, u, u)
	ringQP.MFormLvl(levelQ, levelP, u, u)

	ct0QP := PolyQP{Q: ciphertext.Value[0], P: poolP0}
	ct1QP := PolyQP{Q: ciphertext.Value[1], P: poolP1}

	// ct0 = u*pk0
	// ct1 = u*pk1
	ringQP.MulCoeffsMontgomeryLvl(levelQ, levelP, u, enc.pk.Value[0], ct0QP)
	ringQP.MulCoeffsMontgomeryLvl(levelQ, levelP, u, enc.pk.Value[1], ct1QP)

	// 2*(#Q + #P) NTT
	ringQP.InvNTTLvl(levelQ, levelP, ct0QP, ct0QP)
	ringQP.InvNTTLvl(levelQ, levelP, ct1QP, ct1QP)

	e := PolyQP{Q: poolQ0, P: poolP2}

	enc.gaussianSampler.ReadLvl(levelQ, e.Q)
	ringQP.ExtendBasisSmallNormAndCenter(e.Q, levelP, nil, e.P)
	ringQP.AddLvl(levelQ, levelP, ct0QP, e, ct0QP)

	enc.gaussianSampler.ReadLvl(levelQ, e.Q)
	ringQP.ExtendBasisSmallNormAndCenter(e.Q, levelP, nil, e.P)
	ringQP.AddLvl(levelQ, levelP, ct1QP, e, ct1QP)

	// ct0 = (u*pk0 + e0)/P
	enc.basisextender.ModDownQPtoQ(levelQ, levelP, ct0QP.Q, ct0QP.P, ct0QP.Q)

	// ct1 = (u*pk1 + e1)/P
	enc.basisextender.ModDownQPtoQ(levelQ, levelP, ct1QP.Q, ct1QP.P, ct1QP.Q)

	if ciphertextNTT {

		if !plaintext.Value.IsNTT {
			ringQ.AddLvl(levelQ, ciphertext.Value[0], plaintext.Value, ciphertext.Value[0])
		}

		// 2*#Q NTT
		ringQ.NTTLvl(levelQ, ciphertext.Value[0], ciphertext.Value[0])
		ringQ.NTTLvl(levelQ, ciphertext.Value[1], ciphertext.Value[1])

		if plaintext.Value.IsNTT {
			// ct0 = (u*pk0 + e0)/P + m
			ringQ.AddLvl(levelQ, ciphertext.Value[0], plaintext.Value, ciphertext.Value[0])
		}

	} else {

		if !plaintext.Value.IsNTT {
			ringQ.AddLvl(levelQ, ciphertext.Value[0], plaintext.Value, ciphertext.Value[0])
		} else {
			ringQ.InvNTTLvl(levelQ, plaintext.Value, poolQ0)
			ringQ.AddLvl(levelQ, ciphertext.Value[0], poolQ0, ciphertext.Value[0])
		}
	}

	ciphertext.Value[1].IsNTT = ciphertext.Value[0].IsNTT
	ciphertext.Value[0].Coeffs = ciphertext.Value[0].Coeffs[:levelQ+1]
	ciphertext.Value[1].Coeffs = ciphertext.Value[1].Coeffs[:levelQ+1]
}

func (enc *pkEncryptor) encryptNoP(plaintext *Plaintext, ciphertext *Ciphertext) {
	levelQ := utils.MinInt(plaintext.Level(), ciphertext.Level())

	poolQ0 := enc.poolQ[0]

	ringQ := enc.params.RingQ()

	ciphertextNTT := ciphertext.Value[0].IsNTT

	enc.ternarySampler.ReadLvl(levelQ, poolQ0)
	ringQ.NTTLvl(levelQ, poolQ0, poolQ0)
	ringQ.MFormLvl(levelQ, poolQ0, poolQ0)

	// ct0 = u*pk0
	ringQ.MulCoeffsMontgomeryLvl(levelQ, poolQ0, enc.pk.Value[0].Q, ciphertext.Value[0])
	// ct1 = u*pk1
	ringQ.MulCoeffsMontgomeryLvl(levelQ, poolQ0, enc.pk.Value[1].Q, ciphertext.Value[1])

	if ciphertextNTT {

		// ct1 = u*pk1 + e1
		enc.gaussianSampler.ReadLvl(levelQ, poolQ0)
		ringQ.NTTLvl(levelQ, poolQ0, poolQ0)
		ringQ.AddLvl(levelQ, ciphertext.Value[1], poolQ0, ciphertext.Value[1])

		// ct0 = u*pk0 + e0
		enc.gaussianSampler.ReadLvl(levelQ, poolQ0)

		if !plaintext.Value.IsNTT {
			ringQ.AddLvl(levelQ, poolQ0, plaintext.Value, poolQ0)
			ringQ.NTTLvl(levelQ, poolQ0, poolQ0)
			ringQ.AddLvl(levelQ, ciphertext.Value[0], poolQ0, ciphertext.Value[0])
		} else {
			ringQ.NTTLvl(levelQ, poolQ0, poolQ0)
			ringQ.AddLvl(levelQ, ciphertext.Value[0], poolQ0, ciphertext.Value[0])
			ringQ.AddLvl(levelQ, ciphertext.Value[0], plaintext.Value, ciphertext.Value[0])
		}

	} else {

		ringQ.InvNTTLvl(levelQ, ciphertext.Value[0], ciphertext.Value[0])
		ringQ.InvNTTLvl(levelQ, ciphertext.Value[1], ciphertext.Value[1])

		// ct[0] = pk[0]*u + e0
		enc.gaussianSampler.ReadAndAddLvl(ciphertext.Level(), ciphertext.Value[0])

		// ct[1] = pk[1]*u + e1
		enc.gaussianSampler.ReadAndAddLvl(ciphertext.Level(), ciphertext.Value[1])

		if !plaintext.Value.IsNTT {
			ringQ.AddLvl(levelQ, ciphertext.Value[0], plaintext.Value, ciphertext.Value[0])
		} else {
			ringQ.InvNTTLvl(levelQ, plaintext.Value, poolQ0)
			ringQ.AddLvl(levelQ, ciphertext.Value[0], poolQ0, ciphertext.Value[0])
		}
	}

	ciphertext.Value[1].IsNTT = ciphertext.Value[0].IsNTT

	ciphertext.Value[0].Coeffs = ciphertext.Value[0].Coeffs[:levelQ+1]
	ciphertext.Value[1].Coeffs = ciphertext.Value[1].Coeffs[:levelQ+1]
}

func (enc *skEncryptor) encrypt(plaintext *Plaintext, ciphertext *Ciphertext) {

	ringQ := enc.params.RingQ()

	levelQ := utils.MinInt(plaintext.Level(), ciphertext.Level())

	poolQ0 := enc.poolQ[0]

	ciphertextNTT := ciphertext.Value[0].IsNTT

	ringQ.MulCoeffsMontgomeryLvl(levelQ, ciphertext.Value[1], enc.sk.Value.Q, ciphertext.Value[0])
	ringQ.NegLvl(levelQ, ciphertext.Value[0], ciphertext.Value[0])

	if ciphertextNTT {

		enc.gaussianSampler.ReadLvl(levelQ, poolQ0)

		if plaintext.Value.IsNTT {
			ringQ.NTTLvl(levelQ, poolQ0, poolQ0)
			ringQ.AddLvl(levelQ, ciphertext.Value[0], poolQ0, ciphertext.Value[0])
			ringQ.AddLvl(levelQ, ciphertext.Value[0], plaintext.Value, ciphertext.Value[0])
		} else {
			ringQ.AddLvl(levelQ, poolQ0, plaintext.Value, poolQ0)
			ringQ.NTTLvl(levelQ, poolQ0, poolQ0)
			ringQ.AddLvl(levelQ, ciphertext.Value[0], poolQ0, ciphertext.Value[0])
		}

		ciphertext.Value[0].IsNTT = true
		ciphertext.Value[1].IsNTT = true

	} else {

		if plaintext.Value.IsNTT {
			ringQ.AddLvl(levelQ, ciphertext.Value[0], plaintext.Value, ciphertext.Value[0])
			ringQ.InvNTTLvl(levelQ, ciphertext.Value[0], ciphertext.Value[0])

		} else {
			ringQ.InvNTTLvl(levelQ, ciphertext.Value[0], ciphertext.Value[0])
			ringQ.AddLvl(levelQ, ciphertext.Value[0], plaintext.Value, ciphertext.Value[0])
		}

		enc.gaussianSampler.ReadAndAddLvl(ciphertext.Level(), ciphertext.Value[0])

		ringQ.InvNTTLvl(levelQ, ciphertext.Value[1], ciphertext.Value[1])

		ciphertext.Value[0].IsNTT = false
		ciphertext.Value[1].IsNTT = false

	}

	ciphertext.Value[0].Coeffs = ciphertext.Value[0].Coeffs[:levelQ+1]
	ciphertext.Value[1].Coeffs = ciphertext.Value[1].Coeffs[:levelQ+1]
}

func (enc *encryptor) setKey(key interface{}) Encryptor {
	switch key := key.(type) {
	case *PublicKey:
		if key.Value[0].Q.Degree() != enc.params.N() || key.Value[1].Q.Degree() != enc.params.N() {
			panic("cannot setKey: pk ring degree does not match params ring degree")
		}
		return &pkEncryptor{*enc, key}
	case *SecretKey:
		if key.Value.Q.Degree() != enc.params.N() {
			panic("cannot setKey: sk ring degree does not match params ring degree")
		}
		return &skEncryptor{*enc, key}
	default:
		panic("cannot setKey: key must be either *rlwe.PublicKey or *rlwe.SecretKey")
	}
}

// EncryptRGSW encrypts the input Plaintext and writes the result on the output RGSW ciphertext.
func (enc *pkEncryptor) EncryptRGSW(plaintext *Plaintext, ciphertext *RGSWCiphertext) {
	panic("method not implemented")
}

func (enc *skEncryptor) EncryptRGSW(plaintext *Plaintext, ciphertext *RGSWCiphertext) {

	params := enc.params
	ringQ := params.RingQ()
	ringQP := params.RingQP()
	isNTT := ciphertext.Value[0][0][0][0].Q.IsNTT
	levelQ := ciphertext.LevelQ()
	levelP := ciphertext.LevelP()

	ptTimesP := enc.poolQ[1]

	if plaintext != nil {
		if levelP != -1 {
			var pBigInt *big.Int
			if levelP == params.PCount()-1 {
				pBigInt = params.RingP().ModulusBigint
			} else {
				P := params.RingP().Modulus
				pBigInt = new(big.Int).SetUint64(P[0])
				for i := 1; i < levelP+1; i++ {
					pBigInt.Mul(pBigInt, ring.NewUint(P[i]))
				}
			}

			ringQ.MulScalarBigintLvl(levelQ, plaintext.Value, pBigInt, ptTimesP)
			if !plaintext.Value.IsNTT {
				ringQ.NTTLvl(levelQ, ptTimesP, ptTimesP)
			}

		} else {
			levelP = 0
			if !plaintext.Value.IsNTT {
				ringQ.NTTLvl(levelQ, plaintext.Value, ptTimesP)
			} else {
				ring.CopyLvl(levelQ, plaintext.Value, ptTimesP)
			}
		}
	}

	decompRNS := params.DecompRNS(levelQ, levelP)
	decompBIT := params.DecompBIT(levelQ, levelP)

	var index int
	for j := 0; j < decompBIT; j++ {
		for i := 0; i < decompRNS; i++ {

			enc.encryptZeroSymetricQP(levelQ, levelP, enc.sk.Value, true, isNTT, ciphertext.Value[i][j][0][0], ciphertext.Value[i][j][0][1])
			enc.encryptZeroSymetricQP(levelQ, levelP, enc.sk.Value, true, isNTT, ciphertext.Value[i][j][1][0], ciphertext.Value[i][j][1][1])

			if plaintext != nil {
				for k := 0; k < levelP+1; k++ {

					index = i*(levelP+1) + k

					// It handles the case where nb pj does not divide nb qi
					if index >= levelQ+1 {
						break
					}

					qi := ringQ.Modulus[index]
					p0tmp := ptTimesP.Coeffs[index]
					p1tmp := ciphertext.Value[i][j][0][0].Q.Coeffs[index]
					p2tmp := ciphertext.Value[i][j][1][1].Q.Coeffs[index]

					for w := 0; w < ringQ.N; w++ {
						p1tmp[w] = ring.CRed(p1tmp[w]+p0tmp[w], qi)
						p2tmp[w] = ring.CRed(p2tmp[w]+p0tmp[w], qi)
					}
				}
			}

			ringQP.MFormLvl(levelQ, levelP, ciphertext.Value[i][j][0][0], ciphertext.Value[i][j][0][0])
			ringQP.MFormLvl(levelQ, levelP, ciphertext.Value[i][j][0][1], ciphertext.Value[i][j][0][1])
			ringQP.MFormLvl(levelQ, levelP, ciphertext.Value[i][j][1][0], ciphertext.Value[i][j][1][0])
			ringQP.MFormLvl(levelQ, levelP, ciphertext.Value[i][j][1][1], ciphertext.Value[i][j][1][1])
		}

		ringQ.MulScalar(ptTimesP, 1<<params.LogBase2(), ptTimesP)
	}
}

func (enc *encryptor) encryptZeroSymetricQP(levelQ, levelP int, sk PolyQP, sample, ntt bool, a, b PolyQP) {

	params := enc.params
	ringQP := params.RingQP()

	hasModulusP := a.P != nil && b.P != nil

	if ntt {
		enc.gaussianSampler.ReadLvl(levelQ, a.Q)

		if hasModulusP {
			ringQP.ExtendBasisSmallNormAndCenter(a.Q, levelP, nil, a.P)
		}

		ringQP.NTTLvl(levelQ, levelP, a, a)
	}

	if sample {
		enc.uniformSamplerQ.ReadLvl(levelQ, b.Q)

		if hasModulusP {
			enc.uniformSamplerP.ReadLvl(levelP, b.P)
		}
	}

	ringQP.MulCoeffsMontgomeryAndSubLvl(levelQ, levelP, b, sk, a)

	if !ntt {
		ringQP.InvNTTLvl(levelQ, levelP, a, a)
		ringQP.InvNTTLvl(levelQ, levelP, b, b)

		e := PolyQP{Q: enc.poolQ[0], P: enc.poolP[0]}
		enc.gaussianSampler.ReadLvl(levelQ, e.Q)

		if hasModulusP {
			ringQP.ExtendBasisSmallNormAndCenter(e.Q, levelP, nil, e.P)
		}

		ringQP.AddLvl(levelQ, levelP, a, e, a)
	}
}

func (enc *encryptor) encryptZeroSymetricQ(levelQ int, sk *ring.Poly, sample, ntt bool, a, b *ring.Poly) {

	params := enc.params
	ringQ := params.RingQ()

	if ntt {
		enc.gaussianSampler.ReadLvl(levelQ, a)
		ringQ.NTTLazyLvl(levelQ, a, a)
	}

	if sample {
		enc.uniformSamplerQ.ReadLvl(levelQ, b)
	}

	ringQ.MulCoeffsMontgomeryAndSubLvl(levelQ, b, sk, a)

	if !ntt {
		ringQ.InvNTTLvl(levelQ, a, a)
		ringQ.InvNTTLvl(levelQ, b, b)
		enc.gaussianSampler.ReadAndAddLvl(levelQ, a)
	}
}
