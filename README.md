# What does this do?

[Kubernetes NetworkPolicy](https://kubernetes.io/docs/concepts/services-networking/network-policies/) can only "allow" specific traffic. If a pod matches at least one policy, then only the traffic that is explicitly allowed will be permitted.

The Calico CNI plugin (Container Network Interface) introduces more flexibility, by introducing [their own policies](https://docs.tigera.io/calico/latest/reference/resources/networkpolicy) with an explicit order (via a priority value) and rules that can either allow or deny traffic. However, the same behavior is kept, where all traffic is allowed unless the pod matches at least one policy, then traffic defaults to blocked.

**In other words, the default for pods not matched by a policy is allow all, and the default for pods matched by a policy is deny all. Those are not configurable. This project allows you to change those defaults on a namespace basis.**

# How does it do it?

You can create a Calico NetworkPolicy to set whatever default you want for pods not matched by a user policy (for example: allow traffic within the namespace).

When a Kubernetes NetworkPolicy is created in the namespace, this system will automatically create another Calico NetworkPolicy to apply whatever defaults you want for pods matched by a user policy (for example: deny all).

You can also create a high-priority Calico NetworkPolicy that will take precedence over your defaults and the Kubernetes policies.

# Who needs this?

This is mostly useful for operators of multi-tenant Kubernetes clusters, where you want to let your users create Kubernetes NetworkPolicies and keep their documented behavior, but want to apply more sane defaults and additional controls.
