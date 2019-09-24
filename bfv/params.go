package bfv

import (
	"errors"
	"fmt"
	"github.com/ldsec/lattigo/utils"
	"math"
	"math/bits"
)

// MaxN is the largest supported polynomial modulus degree
const MaxN = 1 << 16

// MaxModuliCount is the largest supported number of 60 moduli in the RNS representation
const MaxModuliCount = 34

// Modulies for 128 security according to http://homomorphicencryption.org/white_papers/security_homomorphic_encryption_white_paper.pdf

var logN11Q56 = []uint64{72057594036879361}

var logN12Q110 = []uint64{36028797014376449, 36028797013327873}

var logN13Q219 = []uint64{36028797014376449, 36028797013327873, 36028797010444289, 18014398506729473}

var logN14Q441 = []uint64{72057594036879361, 36028797014376449, 36028797013327873, 36028797010444289,
	36028797005856769, 36028797001138177, 36028796997599233, 36028796996681729}

var logN15Q885 = []uint64{576460752300015617, 576460752298835969, 576460752298180609, 576460752289923073,
	576460752289529857, 576460752289005569, 576460752286253057, 576460752284418049,
	576460752279306241, 576460752273801217, 576460752272228353, 576460752267509761,
	576460752265543681, 576460752260694017, 576460752259645441}

var logN16Q1770 = []uint64{576460752300015617, 576460752298835969, 576460752298180609, 576460752289923073,
	576460752289529857, 576460752289005569, 576460752286253057, 576460752284418049,
	576460752279306241, 576460752273801217, 576460752272228353, 576460752267509761,
	576460752265543681, 576460752260694017, 576460752259645441, 576460752254795777,
	576460752253747201, 576460752253616129, 576460752250601473, 576460752249683969,
	576460752248635393, 576460752247717889, 576460752246276097, 576460752241033217,
	576460752236445697, 576460752232906753, 576460752223469569, 576460752221765633,
	576460752220192769, 576460752217178113}

var Pi60 = []uint64{576460752308273153, 576460752315482113, 576460752319021057, 576460752319414273, 576460752321642497,
	576460752325705729, 576460752328327169, 576460752329113601, 576460752329506817, 576460752329900033,
	576460752331210753, 576460752337502209, 576460752340123649, 576460752342876161, 576460752347201537,
	576460752347332609, 576460752352837633, 576460752354017281, 576460752355065857, 576460752355459073,
	576460752358604801, 576460752364240897, 576460752368435201, 576460752371187713, 576460752373547009,
	576460752374333441, 576460752376692737, 576460752378003457, 576460752378396673, 576460752380755969,
	576460752381411329, 576460752386129921, 576460752395173889, 576460752395960321, 576460752396091393,
	576460752396484609, 576460752399106049, 576460752405135361, 576460752405921793, 576460752409722881,
	576460752410116097, 576460752411033601, 576460752412082177, 576460752416145409, 576460752416931841,
	576460752421257217, 576460752427548673, 576460752429514753, 576460752435281921, 576460752437248001,
	576460752438558721, 576460752441966593, 576460752449044481, 576460752451141633, 576460752451534849,
	576460752462938113, 576460752465952769, 576460752468705281, 576460752469491713, 576460752472375297,
	576460752473948161, 576460752475389953, 576460752480894977, 576460752483254273, 576460752484827137,
	576460752486793217, 576460752486924289, 576460752492691457, 576460752498589697, 576460752498720769,
	576460752499507201, 576460752504225793, 576460752505405441, 576460752507240449, 576460752507764737,
	576460752509206529, 576460752510124033, 576460752510779393, 576460752511959041, 576460752514449409,
	576460752516284417, 576460752519168001, 576460752520347649, 576460752520609793, 576460752522969089,
	576460752523100161, 576460752524279809, 576460752525852673, 576460752526245889, 576460752526508033,
	576460752532013057, 576460752545120257, 576460752550100993, 576460752551804929, 576460752567402497,
	576460752568975361, 576460752573431809, 576460752580902913, 576460752585490433, 576460752586407937}

// Power of 2 plaintext modulus
var Tpow2 = []uint64{2, 4, 8, 16, 32, 64, 128, 256, 512, 1024, 2048, 4096, 8192, 16384, 32768, 65536, 131072, 262144,
	524288, 1048576, 2097152, 4194304, 8388608, 16777216, 33554432, 67108864, 134217728, 268435456, 536870912,
	1073741824, 2147483648, 4294967296}

// Plaintext modulus allowing batching for the corresponding N in ascending bitsize.
var TBatching = map[uint64][]uint64{
	4096: {40961, 114689, 188417, 417793, 1032193, 2056193, 4169729, 8380417, 16760833, 33538049, 67084289, 134176769,
		268369921, 536813569, 1073692673, 2147377153, 4294828033},
	8192: {65537, 114689, 163841, 1032193, 1785857, 4079617, 8273921, 16760833, 33538049, 67043329, 133857281,
		268369921, 536690689, 1073692673, 2147352577, 4294475777},
	16384: {65537, 163841, 786433, 1769473, 3735553, 8257537, 16580609, 33292289, 67043329, 133857281, 268369921,
		536641537, 1073643521, 2147352577, 4294475777},
	32768: {65537, 786433, 1769473, 3735553, 8257537, 16580609, 33292289, 67043329, 132710401, 268369921, 536608769,
		1073479681, 2147352577, 4293918721},
}

// Parameters represents a given parameter set for the BFV cryptosystem.
type Parameters struct {
	N     uint64
	T     uint64
	Qi    []uint64
	Pi    []uint64
	Sigma float64
}

// DefaultParams is an array default parameters with increasing homomorphic capacity.
// These parameters correspond to 128 bit security level for secret keys in the ternary distribution
// (see //https://projects.csail.mit.edu/HEWorkshop/HomomorphicEncryptionStandard2018.pdf).
var DefaultParams = []Parameters{
	{4096, 65537, logN12Q110, Pi60[len(Pi60)-3:], 3.19},
	{8192, 65537, logN13Q219, Pi60[len(Pi60)-5:], 3.19},
	{16384, 65537, logN14Q441, Pi60[len(Pi60)-9:], 3.19},
	{32768, 65537, logN15Q885, Pi60[len(Pi60)-17:], 3.19},
	//{65536, 786433, logN16Q1770, Pi60[len(Pi60)-34:], 3.19},
}

// Equals compares two sets of parameters for equality
func (p *Parameters) Equals(other *Parameters) bool {
	if p == other {
		return true
	}
	return p.N == other.N && EqualSlice(p.Qi, other.Qi) && EqualSlice(p.Pi, other.Pi) && p.Sigma == other.Sigma
}

// MarshalBinary returns a []byte representation of the parameter set
func (p *Parameters) MarshalBinary() ([]byte, error) {
	if p.N == 0 { // if N is 0, then p is the zero value
		return []byte{}, nil
	}
	b := utils.NewBuffer(make([]byte, 0, 3+((2+len(p.Qi)+len(p.Pi))<<3)))
	b.WriteUint8(uint8(bits.Len64(p.N) - 1))
	b.WriteUint8(uint8(len(p.Qi)))
	b.WriteUint8(uint8(len(p.Pi)))
	b.WriteUint64(p.T)
	b.WriteUint64(uint64(p.Sigma * (1 << 32)))
	b.WriteUint64Slice(p.Qi)
	b.WriteUint64Slice(p.Pi)
	return b.Bytes(), nil
}

// UnMarshalBinary decodes a []byte into a parameter set struct
func (p *Parameters) UnMarshalBinary(data []byte) error {
	if len(data) < 3 {
		return errors.New("invalid parameters encoding")
	}
	b := utils.NewBuffer(data)
	p.N = 1 << uint64(b.ReadUint8())
	if p.N > MaxN {
		return errors.New("polynomial degree is too large")
	}
	lenQi := uint64(b.ReadUint8())
	if lenQi > MaxModuliCount {
		return fmt.Errorf("len(Qi) is larger than %d", MaxModuliCount)
	}
	lenPi := uint64(b.ReadUint8())
	if lenPi > MaxModuliCount {
		return fmt.Errorf("len(Pi) is larger than %d", MaxModuliCount)
	}
	p.T = b.ReadUint64()
	p.Sigma = math.Round((float64(b.ReadUint64())/float64(1<<32))*100) / 100
	p.Qi = make([]uint64, lenQi, lenQi)
	p.Pi = make([]uint64, lenPi, lenPi)
	b.ReadUint64Slice(p.Qi)
	b.ReadUint64Slice(p.Pi)
	return nil
}
