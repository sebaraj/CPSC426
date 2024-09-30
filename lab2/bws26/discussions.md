## CPSC 426 - Lab 1 Discussion

#### Bryan SebaRaj

#### 10/02/2024

#### UUID

The properties of an UUID (universally unique identifier) are that it is a 128-bit number and it is
absolutely unique. The uniqueness of the UUID is guaranteed by the formalized standards that require
the UUID, regardless of language, to be generated using a combination of the current time and the
MAC address of the machine, which itself if unique for the machine within its network, generating
the UUID (for versions 1 & 6 of the OSF DCE standard). There are also other versions that allow for
the use of namespaces and random numbers to generate the UUID.

#### B1.

I've used k8s before, so I know that by default, the pods will be automatically restarted if the pod
running the service is deleted/crashes. After killing the pod, the pod automically restarted as k8s
recognized that the pod had gone down and spun up a new one. This ensures that the service is
(almost) always available, and more resilient to crashing failures.

#### B4.

Welcome! You have chosen user ID 201587 (Bartoletti9899/colinthiel@howe.info)

Their recommended videos are:

1.  bad far by Jaiden Funk
2.  naughty on by Graham Nikolaus
3.  Turquoisezebra: copy by Leanne Hegmann
4.  The aloof raccoon's welfare by Luella Barrows
5.  Hostjump: reboot by Roberto Larkin

#### C3.

With a client pool size of 4, the load distribution is even across the two instances of the
{user|video} services. If only there was only one client within the pool, the load would exclusively
be send to one instance of the user|video service each, as there is only one
connection" which can get load balned by the level 4 load balances in the k9s pod. Meanwhile, if
there are 8 clients the pool, the load would be evenly distributed across the two instnances of the
services. This can be abstracted to the test:

if number_of_clients % number_of_connections_in_pool == 0:
load_distribution = even
else:
load_distribution = uneven
