use std::ops::Mul;
use ark_ff::Field;
use ark_ec::pairing::Pairing;
use crate::utils::{div, mul, evaluate, interpolate};


pub struct KZG<E:Pairing>{
    pub g1 : E::G1, 
    pub g2 :E::G2,
    pub g2_tau: E::G2,
    pub degree: usize,
    pub crs_g1: Vec<E::G1>,
    pub crs_g2: Vec<E::G2>,
}


impl <E:Pairing> KZG<E>{
    pub fn new(g1:E::G1,g2:E::G2,degree:usize)->Self{
        Self{
            g1,
            g2,
            g2_tau: g2.mul(E::ScalarField::ZERO),
            degree,
            crs_g1: vec![],
            crs_g2: vec![],
        }
    }
    pub fn setup(&mut self,secret: E::ScalarField){
        for i in 0..self.degree+1{
            self.crs_g1.push(self.g1.mul(secret.pow(&[i as u64])));
            self.crs_g2.push(self.g2.mul(secret.pow(&[i as u64])));
        }
        self.g2_tau=self.g2.mul(secret);
    }
    pub fn commit(&self,poly:&[E::ScalarField])->E::G1{
        let mut commitment = self.g1.mul(E::ScalarField::ZERO);
        for i in 0..self.degree+1{
            commitment += self.crs_g1[i]*poly[i];
        }
        commitment
    }
    pub fn open(&self, poly: &[E::ScalarField], point: E::ScalarField) -> E::G1 {
        // evaluate the polynomial at point
        let value = evaluate(poly, point);
 
        // initialize denominator
        let denominator = [-point, E::ScalarField::ONE];

        // initialize numerator
        let first = poly[0] - value;
        let rest = &poly[1..];
        let temp: Vec<E::ScalarField> = std::iter::once(first).chain(rest.iter().cloned()).collect();
        let numerator: &[E::ScalarField] = &temp;

        // get quotient by dividing numerator by denominator
        let quotient = div(numerator, &denominator).unwrap();

        // calculate pi as proof (quotient multiplied by CRS)
        let mut pi = self.g1.mul(E::ScalarField::ZERO);
        for i in 0..quotient.len() {
            pi += self.crs_g1[i] * quotient[i];
        }

        // return pi
        pi
    }

}


