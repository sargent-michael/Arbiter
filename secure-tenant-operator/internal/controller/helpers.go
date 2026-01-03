package controller

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func resourceMustParse(s string) resource.Quantity {
	return resource.MustParse(s)
}

func protoPtr(p string) *corev1.Protocol {
	pp := corev1.Protocol(p)
	return &pp
}

func intstrPtr(i int) *intstr.IntOrString {
	v := intstr.FromInt(i)
	return &v
}
