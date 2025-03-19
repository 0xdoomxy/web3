package main

import (
	"crypto/rand"
	"math/big"
	"testing"

	bls12381 "github.com/kilic/bls12-381"
	"github.com/stretchr/testify/assert"
)

func TestBLSMultiSign(t *testing.T) {
	g1 := bls12381.NewG1()
	//随机生成私钥,私钥要小于G1的阶
	privateKey, err := rand.Int(rand.Reader, bls12381.NewG1().Q())
	if err != nil {
		panic(err)
	}
	//生成成员秘钥,这里的质数必须足够大,如果不符合要求则会在gcd()的时候出现问题，我这里直接使用了g1的阶。
	secrets := generateShares(privateKey, 3, 8, bls12381.NewG1().Q())
	assert.Equal(t, 0, lagrangeInterpolation(secrets, bls12381.NewG1().Q()).Cmp(privateKey), "恢复秘钥失败")
	//使用部分秘钥对曲线hash进行标量乘法
	G := g1.One()
	pks := make([]*bls12381.PointG1, 0, len(secrets))
	for i := 0; i < len(secrets); i++ {
		pk := g1.New()
		g1.MulScalar(pk, G, new(bls12381.Fr).FromBytes(secrets[i].Value.Bytes()))
		pks = append(pks, pk)
	}
	msg := "Hello world!"
	g2 := bls12381.NewG2()
	msgPoint, err := g2.HashToCurve([]byte(msg), []byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_"))
	if err != nil {
		panic(err)
	}
	var msgSignPoint = make([]*bls12381.PointG2, 0, len(secrets))
	msgSignAgg := g2.New()
	for i := 0; i < len(secrets); i++ {
		msgSign := g2.New()
		g2.MulScalar(msgSign, msgPoint, new(bls12381.Fr).FromBytes(secrets[i].Value.Bytes()))
		msgSignPoint = append(msgSignPoint, msgSign)
	}
	var defaultFr = make([]*big.Int, 0, len(secrets))
	for i := 0; i < len(secrets); i++ {
		defaultFr = append(defaultFr, new(big.Int).SetInt64(1))
	}
	var publicKeyAgg = g1.New()
	//构造双线性映射 e(G,signA*signB*signc) == e(pkA*pkB*pkC,Hash(msg))
	g2.MultiExpBig(msgSignAgg, msgSignPoint, defaultFr)
	g1.MultiExpBig(publicKeyAgg, pks, defaultFr)
	e1 := bls12381.NewEngine().AddPair(G, msgSignAgg)
	e2 := bls12381.NewEngine().AddPair(publicKeyAgg, msgPoint)
	assert.Equal(t, true, e1.Result().Equal(e2.Result()), "BLS多签失败")

}

type SecretShare struct {
	ID    int64
	Value *big.Int
}

// 利用代数基本定理实现t-n秘钥分发,总共分发n个秘钥，只需要t个成员秘钥即可恢复原秘钥
func generateShares(secret *big.Int, t, n int64, prime *big.Int) []*SecretShare {
	coeffs := make([]*big.Int, t)
	coeffs[0] = secret

	for i := int64(1); i < t; i++ {
		n, _ := rand.Prime(rand.Reader, 256)
		coeffs[i] = new(big.Int).Set(n)
	}

	shares := make([]*SecretShare, n)
	for i := int64(1); i <= n; i++ {
		fx := new(big.Int)
		for j := int64(0); j < t; j++ {
			// f(i) = a0 + a1 * i + a2 * i^2 + ... + at-1 * i^(t-1)
			term := new(big.Int).Mul(coeffs[j], new(big.Int).Exp(big.NewInt(i), big.NewInt(j), prime))
			fx.Add(fx, term)
			fx.Mod(fx, prime)
		}
		shares[i-1] = &SecretShare{ID: i, Value: fx}
	}
	return shares
}

func lagrangeInterpolation(shares []*SecretShare, prime *big.Int) *big.Int {
	n := len(shares)
	result := new(big.Int)

	for i := 0; i < n; i++ {
		li := new(big.Int).SetInt64(1)
		denominator := new(big.Int).SetInt64(1)

		for j := 0; j < n; j++ {
			if i != j {
				// L_i(x) = (x - x_j) / (x_i - x_j)
				li.Mul(li, new(big.Int).Sub(big.NewInt(0), big.NewInt(shares[j].ID)))                              // (0 - x_j)
				denominator.Mul(denominator, new(big.Int).Sub(big.NewInt(shares[i].ID), big.NewInt(shares[j].ID))) // (x_i - x_j)
			}
		}

		inv := new(big.Int).ModInverse(denominator, prime)
		// L_i(x) = (x - x_j) / (x_i - x_j)
		li.Mul(li, inv)

		li.Mul(li, shares[i].Value)
		result.Add(result, li)
		result.Mod(result, prime)
	}

	return result
}
func TestBLSSign(t *testing.T) {
	//初始化群G1
	g1 := bls12381.NewG1()
	//随机生成私钥,私钥要小于G1的阶
	privateKey, err := rand.Int(rand.Reader, bls12381.NewG1().Q())
	if err != nil {
		panic(err)
	}
	//获取G1的基点
	G := g1.One()
	// 通过 privateKey * G 计算公钥
	publicKey := g1.New()
	g1.MulScalar(publicKey, G, new(bls12381.Fr).FromBytes(privateKey.Bytes()))
	// 对消息进行哈希,映射到G2点
	msg := []byte("hello world!")
	g2 := bls12381.NewG2()
	hashPoint, err := g2.HashToCurve(msg, []byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_POP_"))
	if err != nil {
		panic(err)
	}
	//通过 privateKey * hashPoint 计算签名
	hashsignPoint := g2.New()
	g2.MulScalar(hashsignPoint, hashPoint, new(bls12381.Fr).FromBytes(privateKey.Bytes()))
	//生成 e(G,hashsignPoint) 和 e(publicKey,hashPoint)
	e1 := bls12381.NewEngine().AddPair(publicKey, hashPoint)
	e2 := bls12381.NewEngine().AddPair(G, hashsignPoint)
	//验证e1 和 e2 是否相等
	assert.Equal(t, true, e1.Result().Equal(e2.Result()), "BLS签名失败")
}
