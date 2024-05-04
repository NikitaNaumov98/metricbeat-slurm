## Installation

* Clone Elastic Beats repository
  * `git clone https://github.com/elastic/beats`
* Clone this repository into another directory
  * `git clone https://github.com/NikitaNaumov98/metricbeat-slurm`
* Copy slurm module into cloned Beats repository
  * `cp -r *location of metricbeat-slurm repository*/metricbeat/module/slurm *location of beats repository*/metricbeat/module/slurm`
* Add required dependency
  * `go get github.com/mitchellh/go-ps`
* Change into metricbeat directory and run `mage update`
* Run `mage build`

If module fails to work, try adding this line to the `metricbeat.yml` file:
`seccomp.enabled: false`
